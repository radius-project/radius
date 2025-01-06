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

package create

import (
	"context"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resource-type create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [input]",
		Short: "Create or update a resource type",
		Long: `Create or update a resource type from a resource provider manifest.
		
Resource types are user defined types such as  'Mycompany.Messaging/plaid'. 

Creating a resource type defines a new type that can be used in applications. 

Input can be passed in using a JSON or YAML file using the --from-file option.
`,
		Example: `
# Create a resource type from YAML file
rad resource-type create myType --from-file /path/to/input.yaml

# Create a resource type from JSON file
rad resource-type create myType --from-file /path/to/input.json
`,
		Args: cobra.ExactArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddFromFileFlagVar(cmd, &runner.ResourceProviderManifestFilePath)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourceprovider create` command.
type Runner struct {
	UCPClientFactory *v20231001preview.ClientFactory
	ConfigHolder     *framework.ConfigHolder
	Output           output.Interface
	Format           string
	Workspace        *workspaces.Workspace

	ResourceProviderManifestFilePath string
	ResourceProvider                 *manifest.ResourceProvider
	ResourceTypeName                 string
	Logger                           func(format string, args ...any)
}

// NewRunner creates an instance of the runner for the `rad resourceprovider create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Logger: func(format string, args ...any) {
			output.LogInfo(format, args...)
		},
	}
}

// Validate runs validation for the `rad resourceprovider create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	resourceTypeName, err := cli.RequireResourceTypeNameArgs(cmd, args)
	if err != nil {
		return err
	}
	r.ResourceTypeName = resourceTypeName

	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	r.ResourceProvider, err = manifest.ReadFile(r.ResourceProviderManifestFilePath)
	if err != nil {
		return err
	}

	resourcesTypes := r.ResourceProvider.Types
	if _, ok := resourcesTypes[r.ResourceTypeName]; !ok {
		return clierrors.Message("Resource type %q not found in the manifest", r.ResourceTypeName)
	}

	return nil
}

// Run runs the `rad resourcetype create` command.
func (r *Runner) Run(ctx context.Context) error {
	// Initialize the client factory if it hasn't been set externally.
	// This allows for flexibility where a test UCPClientFactory can be set externally during testing.
	if r.UCPClientFactory == nil {
		err := r.initializeClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
	}

	// If resource provider does not exist or if there is any other error, first register the resource provider.
	response, err := r.UCPClientFactory.NewResourceTypesClient().Get(ctx, "local", r.ResourceProvider.Name, r.ResourceTypeName, nil)
	if err != nil {
		r.Output.LogInfo("Resource provider %q not found.", r.ResourceProvider.Name)
		if err := manifest.RegisterFile(ctx, r.UCPClientFactory, "local", r.ResourceProviderManifestFilePath, r.Logger); err != nil {
			return err
		}
	} else {
		r.Output.LogInfo("Resource provider %q found. Registering resource type %q.", r.ResourceProvider.Name, r.ResourceTypeName)
		if err := manifest.RegisterType(ctx, r.UCPClientFactory, "local", r.ResourceProviderManifestFilePath, r.ResourceTypeName, r.Logger); err != nil {
			return err
		}
	}

	// Add a blank line before printing the result.
	r.Output.LogInfo("")

	err = r.Output.WriteFormatted(r.Format, response, common.GetResourceTypeTableFormat())
	if err != nil {
		return err
	}

	return nil
}

func (r *Runner) initializeClientFactory(ctx context.Context, workspace *workspaces.Workspace) error {
	connection, err := workspace.Connect(ctx)
	if err != nil {
		return err
	}

	clientOptions := sdk.NewClientOptions(connection)

	clientFactory, err := v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, clientOptions)
	if err != nil {
		return err
	}

	r.UCPClientFactory = clientFactory
	return nil
}
