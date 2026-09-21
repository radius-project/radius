/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package publishextension

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/radius-project/radius/bicep-tools/generator"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/process"
	"github.com/spf13/cobra"
)

var numericRegistryHost = regexp.MustCompile(`^(0[xX][0-9a-fA-F]+|[0-9]+)(\.(0[xX][0-9a-fA-F]+|[0-9]+)){0,3}$`)

// NewCommand creates a new instance of the `rad bicep publish-extension` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "publish-extension",
		Short: "Generate or publish a Bicep extension for a set of resource types.",
		Long: `Generate or publish a Bicep extension for a set of resource types.
This command compiles a set of resource types (resource provider manifest) into a Bicep extension for local use or distribution.

Bicep extensions enable extensibility for the Bicep language. This command can be used to generate and distribute Bicep support for resource types authored by users. Bicep extensions can be distributed using Open Container Initiative (OCI) registry, such as Azure Container Registry, Docker Hub, or GitHub Container Registry. See https://learn.microsoft.com/en-us/azure/azure-resource-manager/bicep/bicep-extension for more information on Bicep extensions.

Once an extension is been generated, it can be used locally or published to a container registry for distribution depending on the target specified.

When publishing to an OCI registry it is expected the user runs docker login (or similar command) and has the proper permission to push to the target OCI registry.

Publishing to generic OCI registries requires Bicep v0.45.6 or later. This command enables OCI support for non-loopback registry targets without modifying the user's bicepconfig.json. Loopback and non-canonical numeric registry hosts retain the caller's OCI setting to preserve the existing transport: Bicep uses plain HTTP for loopback when OCI is explicitly enabled. Local file targets are resolved relative to the current directory.
		`,
		Example: `
# Generate a Bicep extension to a local file
rad bicep publish-extension --from-file ./Example.Provider.yaml --target ./output.tgz

# Publish a Bicep extension to a container registry
rad bicep publish-extension --from-file ./Example.Provider.yaml --target br:ghcr.io/myregistry/example-provider:v1
		`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddFromFileFlagVar(cmd, &runner.ResourceProviderManifestFilePath)
	_ = cmd.MarkFlagRequired("from-file")
	_ = cmd.MarkFlagFilename("from-file", "yaml", "json")

	cmd.Flags().StringVar(&runner.Target, "target", "", "The destination path file or OCI registry path. OCI registry paths use the format 'br:HOST/PATH:TAG'.")
	_ = cmd.MarkFlagRequired("target")
	cmd.Flags().BoolVar(&runner.Force, "force", false, "Overwrite the target extension if it exists.")
	return cmd, runner
}

// Runner is the runner implementation for the `rad bicep publish-extension` command.
type Runner struct {
	Output output.Interface

	ResourceProvider                 *manifest.ResourceProvider
	ResourceProviderManifestFilePath string
	Target                           string
	Force                            bool
}

// NewRunner creates a new instance of the `rad bicep publish-extension` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Output: factory.GetOutput(),
	}
}

// Validate validates the `rad bicep publish-extension` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// We read the resource provider manifest upfront to ensure it exists and is valid.
	//
	// The validation we implement in the `rad` CLI is the source of truth for the manifest. The
	// manifest-to-bicep-extension tool does minimal validation, so we want to catch any issues
	// early.
	rp, err := manifest.ReadFile(r.ResourceProviderManifestFilePath)
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to read resource provider %q", r.ResourceProviderManifestFilePath)
	}

	r.ResourceProvider = rp

	return nil
}

// Run runs the `rad bicep publish-extension` command.
func (r *Runner) Run(ctx context.Context) error {
	// This command performs three steps:
	// 1. Run the generator (bicep-tools/generator) to build the Bicep extension "index"
	// 2. We use `bicep publish-extension` to publish the extension "index" to the "target"
	// 3. We can clean up the "index" directory after publishing.

	temp, err := os.MkdirTemp("", "bicep-extension-*")
	if err != nil {
		return err
	}

	defer os.RemoveAll(temp)

	err = generateBicepExtensionIndex(ctx, r.ResourceProviderManifestFilePath, temp)
	if err != nil {
		return err
	}

	err = publishExtension(ctx, temp, r.Target, r.Force)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Successfully published Bicep extension %q to %q", r.ResourceProviderManifestFilePath, r.Target)
	return nil
}

func generateBicepExtensionIndex(ctx context.Context, inputFilePath string, outputDirectoryPath string) error {
	return generator.RunGenerate(inputFilePath, outputDirectoryPath)
}

func publishExtension(ctx context.Context, inputDirectoryPath string, target string, force bool) error {
	callerDirectory, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to determine the publishing directory: %w", err)
	}

	bicepFilePath, err := bicep.GetBicepFilePath()
	if err != nil {
		return err
	}

	bicepFilePath, err = filepath.Abs(bicepFilePath)
	if err != nil {
		return fmt.Errorf("failed to resolve the Bicep executable path: %w", err)
	}
	inputDirectoryPath, err = filepath.Abs(inputDirectoryPath)
	if err != nil {
		return fmt.Errorf("failed to resolve the extension directory: %w", err)
	}

	// Match Bicep's publish-extension target classification, including its handling of local paths.
	registryTarget := strings.HasPrefix(target, "br:") || strings.HasPrefix(target, "ts:")
	if !registryTarget {
		target, err = filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("failed to resolve the extension target: %w", err)
		}
	}

	// bicep publish-extension <temp>/index.json --target <target>
	args := []string{
		"publish-extension",
		filepath.Join(inputDirectoryPath, "index.json"),
		"--target", target,
	}

	if force {
		args = append(args, "--force")
	}

	cmd := process.CommandContext(ctx, bicepFilePath, args...)
	if registryTarget && !preserveRegistryTransport(target) {
		if err := writePublishConfig(callerDirectory, inputDirectoryPath); err != nil {
			return err
		}

		// Registry targets discover configuration from cwd, not from the generated index.
		// Local and loopback targets keep their original configuration discovery and transport.
		cmd.Dir = inputDirectoryPath
		cmd.Env = cmd.Environ()
		for _, name := range []string{"DOCKER_CONFIG", "AZURE_CONFIG_DIR", "AZURE_FEDERATED_TOKEN_FILE", "AZURE_CLIENT_CERTIFICATE_PATH", "SSL_CERT_FILE"} {
			value := os.Getenv(name)
			if strings.TrimSpace(value) == "" || filepath.IsAbs(value) {
				continue
			}
			absolutePath, err := filepath.Abs(value)
			if err != nil {
				return fmt.Errorf("failed to resolve %s for Bicep: %w", name, err)
			}
			cmd.Env = append(cmd.Env, name+"="+absolutePath)
		}
		for _, name := range []string{"PATH", "SSL_CERT_DIR"} {
			value, exists := os.LookupEnv(name)
			if !exists {
				continue
			}
			paths := filepath.SplitList(value)
			for i, path := range paths {
				paths[i], err = filepath.Abs(path)
				if err != nil {
					return fmt.Errorf("failed to resolve %s for Bicep: %w", name, err)
				}
			}
			cmd.Env = append(cmd.Env, name+"="+strings.Join(paths, string(os.PathListSeparator)))
		}
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to publish Bicep extension")
	}

	return nil
}

// Bicep's OCI transport forces plain HTTP for loopback. Preserve the caller's
// choice there, including numeric IPv4 spellings accepted by .NET but not netip.
func preserveRegistryTransport(target string) bool {
	reference, ok := strings.CutPrefix(target, "br:")
	if !ok {
		return false
	}
	registry, err := url.Parse("https://" + reference)
	if err != nil {
		// Leave malformed references for Bicep to diagnose.
		return false
	}
	host := registry.Hostname()
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.WithZone("")
		if address.Is4In6() {
			// .NET only treats the mapping of 127.0.0.1 as IPv6 loopback.
			return address.Unmap() == netip.AddrFrom4([4]byte{127, 0, 0, 1})
		}
		return address.IsLoopback()
	}
	return numericRegistryHost.MatchString(host)
}

func writePublishConfig(callerDirectory, outputDirectory string) error {
	config, err := readPublishConfig(callerDirectory)
	if err != nil {
		return err
	}

	features := map[string]json.RawMessage{}
	if value, ok := config["experimentalFeaturesEnabled"]; ok {
		if err := json.Unmarshal(value, &features); err != nil {
			return fmt.Errorf("failed to read Bicep experimental features: %w", err)
		}
		if features == nil {
			return errors.New("experimentalFeaturesEnabled in Bicep configuration must be a JSON object")
		}
	}
	features["ociEnabled"] = json.RawMessage("true")
	config["experimentalFeaturesEnabled"], err = json.Marshal(features)
	if err != nil {
		return fmt.Errorf("failed to encode Bicep experimental features: %w", err)
	}

	data, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to encode the publishing configuration: %w", err)
	}
	if err := os.WriteFile(filepath.Join(outputDirectory, "bicepconfig.json"), data, 0600); err != nil {
		return fmt.Errorf("failed to write the publishing configuration: %w", err)
	}
	return nil
}

func readPublishConfig(directory string) (map[string]json.RawMessage, error) {
	for {
		filename := filepath.Join(directory, "bicepconfig.json")
		data, err := os.ReadFile(filename)
		if err == nil {
			data, err = stripJSONComments(data)
			if err != nil {
				return nil, fmt.Errorf("failed to parse Bicep configuration %q: %w", filename, err)
			}
			var config map[string]json.RawMessage
			if err := json.Unmarshal(data, &config); err != nil {
				return nil, fmt.Errorf("failed to parse Bicep configuration %q: %w", filename, err)
			}
			if config == nil {
				return nil, fmt.Errorf("expected a JSON object in Bicep configuration %q", filename)
			}
			return config, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("failed to read Bicep configuration %q: %w", filename, err)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return map[string]json.RawMessage{}, nil
		}
		directory = parent
	}
}

// Bicep accepts JSON comments and a leading UTF-8 BOM, but not trailing commas.
// Replace comments with whitespace so encoding/json still rejects malformed input.
func stripJSONComments(data []byte) ([]byte, error) {
	data = bytes.Clone(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	inString := false
	for i := 0; i < len(data); i++ {
		if inString {
			switch data[i] {
			case '\\':
				i++
			case '"':
				inString = false
			}
			continue
		}
		if data[i] == '"' {
			inString = true
			continue
		}
		if data[i] != '/' || i+1 == len(data) {
			continue
		}
		switch data[i+1] {
		case '/':
			for ; i < len(data) && data[i] != '\n' && data[i] != '\r'; i++ {
				data[i] = ' '
			}
		case '*':
			data[i], data[i+1] = ' ', ' '
			i += 2
			for ; i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/'); i++ {
				if data[i] != '\n' && data[i] != '\r' {
					data[i] = ' '
				}
			}
			if i+1 >= len(data) {
				return nil, errors.New("unterminated JSON comment")
			}
			data[i], data[i+1] = ' ', ' '
			i++
		}
	}
	return data, nil
}
