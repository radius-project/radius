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

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resource-type create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [input]",
		Short: "Create or update a resource type",
		Long: `Create or update a resource type from a resource type manifest.
	
	Resource types are user defined types such as 'Mycompany.Messaging/plaid'.
	
	Creating a resource type defines a new type that can be used in applications.
	
	Input can be passed in using a JSON or YAML file using the --from-file option.

	resource-type name argument is optional. If specified, only the specified type is created/updated. 
	If not specified, all resource types in the referred file are created/updated.
	`,
		Example: `
# Create a resource type from YAML file
rad resource-type create myType --from-file /path/to/input.yaml

# Create a resource type from JSON file
rad resource-type create myType --from-file /path/to/input.json

# Create all resource type from YAML file
rad resource-type create  --from-file /path/to/input.yaml
 
# Create all resource type from JSON file
rad resource-type create myType --from-file /path/to/input.json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddFromFileFlagVar(cmd, &runner.ResourceProviderManifestFilePath)

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resource-type create` command.
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

// NewRunner creates an instance of the runner for the `rad resource-type create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		Logger: func(format string, args ...any) {
			output.LogInfo(format, args...)
		},
	}
}

// Validate runs validation for the `rad resource-type create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	resourceTypeName := cli.ReadResourceTypeNameArgs(cmd, args)
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
	if r.ResourceTypeName != "" {
		_, ok := resourcesTypes[r.ResourceTypeName]
		if !ok {
			return clierrors.Message("Resource type %q not found in the manifest", r.ResourceTypeName)
		}
	}

	return nil
}

// Run runs the `rad resource-type create` command.
func (r *Runner) Run(ctx context.Context) error {
	// Initialize the client factory if it hasn't been set externally.
	// This allows for flexibility where a test UCPClientFactory can be set externally during testing.
	if r.UCPClientFactory == nil {
		clientFactory, err := cmd.InitializeClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.UCPClientFactory = clientFactory
	}

	if r.ResourceTypeName == "" {
		r.Output.LogInfo("No resource type name provided. Registering all resource types in the manifest for resource provider %q.", r.ResourceProvider.Name)

		registerErr := manifest.RegisterResourceProvider(ctx, r.UCPClientFactory, "local", *r.ResourceProvider, r.Logger)
		if registerErr != nil {
			return registerErr
		}
	} else {
		r.Output.LogInfo("Registering resource type %q for resource provider %q.", r.ResourceTypeName, r.ResourceProvider.Name)

		_, err := r.UCPClientFactory.NewResourceProvidersClient().Get(ctx, "local", r.ResourceProvider.Name, nil)
		if err != nil {
			if clients.Is404Error(err) {
				for key := range r.ResourceProvider.Types {
					if key != r.ResourceTypeName {
						delete(r.ResourceProvider.Types, key)
					}
				}
				registerErr := manifest.RegisterResourceProvider(ctx, r.UCPClientFactory, "local", *r.ResourceProvider, r.Logger)
				if registerErr != nil {
					return err
				}
			} else {
				return err
			}

			r.Output.LogInfo("Resource type %s/%s created successfully", r.ResourceProvider.Name, r.ResourceTypeName)
		} else {
			registerErr := manifest.RegisterType(ctx, r.UCPClientFactory, "local", r.ResourceProviderManifestFilePath, r.ResourceTypeName, r.Logger)
			if registerErr != nil {
				return err
			}
		}
	}

	return nil
}
