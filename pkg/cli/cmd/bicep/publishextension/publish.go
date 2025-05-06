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
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"

	"github.com/spf13/cobra"
)

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
		`,
		Example: `
# Generate a Bicep extension to a local file
rad bicep publish-extension --from-file ./Example.Provider.yaml --target ./output.tgz

# Publish a Bicep extension to a container registry
bicep publish-extension ./Example.Provider.yaml --target br:ghcr.io/myregistry/example-provider:v1
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
	// This command ties together two separate shell commands:
	// 1. We use NPX to run https://github.com/radius-project/bicep-tools/tree/main/packages/manifest-to-bicep-extension
	//       - This generates a Bicep extension "index"
	// 2. We use `bicep publish-extension` to publish the extension "index" to the "target"
	//
	// 3. We can clean up the "index" directory after publishing.

	_, err := exec.LookPath("npx")
	if errors.Is(err, exec.ErrNotFound) {
		return clierrors.Message("The command 'npx' was not found on the PATH. Please install Node.js 16+ to use this command.")
	}

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
	// npx @radius-project/manifest-to-bicep-extension@alpha generate <resource provider> <temp>
	args := []string{
		"@radius-project/manifest-to-bicep-extension@alpha",
		"generate",
		inputFilePath,
		outputDirectoryPath,
	}
	cmd := exec.CommandContext(ctx, "npx", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to generate Bicep extension")
	}

	return nil
}

func publishExtension(ctx context.Context, inputDirectoryPath string, target string, force bool) error {
	bicepFilePath, err := bicep.GetBicepFilePath()
	if err != nil {
		return err
	}

	// rad-bicep publish-extension <temp>/index.json --target <target>
	args := []string{
		"publish-extension",
		filepath.Join(inputDirectoryPath, "index.json"),
		"--target", target,
	}

	if force {
		args = append(args, "--force")
	}

	cmd := exec.CommandContext(ctx, bicepFilePath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to publish Bicep extension")
	}

	return nil
}
