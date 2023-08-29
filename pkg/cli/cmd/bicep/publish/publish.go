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

package bicep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	credentials "github.com/oras-project/oras-credentials-go"
	"github.com/radius-project/radius/pkg/cli/bicep"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"

	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/memory"
	"oras.land/oras-go/v2/registry"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/retry"
)

const (
	layerMediaType  = "application/vnd.ms.bicep.module.layer.v1+json"
	configMediaType = "application/vnd.ms.bicep.module.config.v1+json"
)

type destination struct {
	host string
	repo string
	tag  string
}

// NewCommand creates an instance of the command and runner for the `rad bicep publish` command.
//

// "NewCommand" creates a new Cobra command and a Runner object, sets up the command's flags and usage information, and
// returns the command and the Runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish a Bicep file to an OCI registry.",
		Long: `Publish a Bicep file to an OCI registry.
This command compiles and publishes a local Bicep file to a remote Open Container Initiative (OCI) registry, such as Azure Container Registry, Docker Hub, or GitHub Container Registry, to later be used as a Bicep registry or for Radius Recipes.
Before publishing, it is expected the user runs docker login (or similar command) and has the proper permission to push to the target OCI registry.
For more information on Bicep modules visit https://learn.microsoft.com/azure/azure-resource-manager/bicep/modules
		`,
		Example: `
# Publish a Bicep file to an Azure container registry
rad bicep publish --file ./redis-test.bicep --target br:myregistry.azurecr.io/redis-test:v1
		`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().String("file", "", "path to the local Bicep file, relative to the current working directory.")
	_ = cmd.MarkFlagRequired("file")
	cmd.Flags().String("target", "", "remote OCI registry path, in the format 'br:HOST/PATH:TAG'.")
	_ = cmd.MarkFlagRequired("target")

	return cmd, runner
}

// Runner is the runner implementation for the `rad bicep publish` command.
type Runner struct {
	Bicep             bicep.Interface
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface

	File          string
	Target        string
	Destination   *destination
	Template      map[string]any
	TemplateBytes []byte
}

// NewRunner creates a new instance of the `rad bicep publish` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		Bicep:             factory.GetBicep(),
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad bicep publish` command.
//

// Runner.Validate parses the command line flags and sets the File and Target fields of the Runner struct, returning an
// error if the target flag is not in the expected format.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	file, err := cmd.Flags().GetString("file")
	if err != nil {
		return err
	}
	r.File = file

	target, err := cmd.Flags().GetString("target")
	if err != nil {
		return err
	}
	if !strings.HasPrefix(target, "br:") {
		return clierrors.Message("Invalid target %q. The target must be in the format 'br:HOST/PATH:TAG'.", target)
	}

	r.Target = strings.TrimPrefix(target, "br:")

	return nil
}

// Run runs the `rad bicep publish` command.
//

// The Run function prepares a Bicep template, extracts the destination, publishes the template to the target, and logs
// a success message if no errors are encountered. An error is returned if any of the steps fail.
func (r *Runner) Run(ctx context.Context) error {
	template, err := r.Bicep.PrepareTemplate(r.File)
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to prepare Bicep file %q.", r.File)
	}
	r.Template = template

	jsonStr, err := json.Marshal(r.Template)
	if err != nil {
		return err
	}
	r.TemplateBytes = []byte(jsonStr)

	dest, err := r.extractDestination()
	if err != nil {
		return err
	}
	r.Destination = dest

	err = r.publish(ctx)
	if err != nil {
		return clierrors.MessageWithCause(err, "Failed to publish Bicep file %q to %q.", r.File, r.Target)
	}

	r.Output.LogInfo("Successfully published Bicep file %q to %q", r.File, r.Target)
	return nil
}

func (r *Runner) publish(ctx context.Context) error {
	// Prepare Source
	src, err := r.prepareSource(ctx)
	if err != nil {
		return err
	}

	// Prepare Destination
	dst, err := r.prepareDestination(ctx)
	if err != nil {
		return err
	}

	desc, err := oras.Copy(ctx, src, r.Destination.tag, dst, r.Destination.tag, oras.DefaultCopyOptions)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Pushed to %s:%s@%s\n", r.Destination.host, r.Destination.repo, desc.Digest)
	return nil
}

// prepareSource prepares the source for the publish operation
func (r *Runner) prepareSource(ctx context.Context) (*memory.Store, error) {
	src := memory.New()

	// Push layer blob
	layerDesc, err := pushBlob(ctx, layerMediaType, r.TemplateBytes, src)
	if err != nil {
		return nil, err
	}

	// Push config blob
	configDesc, err := pushBlob(ctx, configMediaType, nil, src)
	if err != nil {
		return nil, err
	}

	// Generate manifest blob
	manifestBlob, err := generateManifestContent(configDesc, layerDesc) // generate a image manifest
	if err != nil {
		return nil, err
	}

	// Push manifest blob
	manifestDesc, err := pushBlob(ctx, ocispec.MediaTypeImageManifest, manifestBlob, src) // push manifest blob
	if err != nil {
		return nil, err
	}

	// Tag manifest
	err = src.Tag(ctx, manifestDesc, r.Destination.tag)
	if err != nil {
		return nil, err
	}

	return src, nil
}

func (r *Runner) prepareDestination(ctx context.Context) (*remote.Repository, error) {
	// Create a new credential store from Docker to get local credentials
	ds, err := credentials.NewStoreFromDocker(credentials.StoreOptions{
		AllowPlaintextPut: true,
	})
	if err != nil {
		return nil, err
	}

	dst, err := remote.NewRepository(r.Destination.host + "/" + r.Destination.repo)
	if err != nil {
		return nil, err
	}

	dst.Client = &auth.Client{
		Client:     retry.DefaultClient,
		Cache:      auth.DefaultCache,
		Credential: ds.Get,
	}

	return dst, nil
}

// extractDestination extracts the host, repo, and tag from the target
func (r *Runner) extractDestination() (*destination, error) {
	ref, err := registry.ParseReference(r.Target)
	if err != nil {
		return nil, err
	}

	host := ref.Host()
	// This check is needed because by default `docker.io` is redirected to `registry-1.docker.io` in oras client.
	// And we would like to use `index.docker.io` as the Host.
	// Please see: https://github.com/oras-project/oras-go/blob/main/registry/reference.go#L236
	if host == "docker.io" || host == "registry-1.docker.io" || host == "" {
		host = "index.docker.io"
	}

	repo := ref.Repository
	tag := ref.Reference

	if host == "" || repo == "" || tag == "" {
		return nil, fmt.Errorf("invalid target %q", r.Target)
	}

	return &destination{
		host,
		repo,
		tag,
	}, nil
}

// pushBlob pushes a blob to the registry target
func pushBlob(ctx context.Context, mediaType string, blob []byte, target oras.Target) (desc ocispec.Descriptor, err error) {
	desc = ocispec.Descriptor{
		MediaType: mediaType,
		Digest:    digest.FromBytes(blob),
		Size:      int64(len(blob)),
	}
	return desc, target.Push(ctx, desc, bytes.NewReader(blob))
}

// generateManifestContent generates a manifest content based on the config and layers descriptors
func generateManifestContent(config ocispec.Descriptor, layers ...ocispec.Descriptor) ([]byte, error) {
	content := ocispec.Manifest{
		Config:    config,
		Layers:    layers,
		Versioned: specs.Versioned{SchemaVersion: 2},
	}
	return json.Marshal(content)
}
