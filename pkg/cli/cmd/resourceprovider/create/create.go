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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourceprovider/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/to"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the `rad resourceprovider create` command and runner.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [input]",
		Short: "Create or update a resource provider",
		Long: `Create or update a resource provider
		
Resource providers are the entities that implement resource types such as 'Applications.Core/containers'. Resource providers can be defined, registered, and unregistered by users.

Creating a resource provider defines new resource types that can be used in applications.

Input can be passed in using a JSON or YAML file using the --from-file option.
`,
		Example: `
# Create a resource provider from YAML input
rad resource-provider create --from-file /path/to/input.yaml

# Create a resource provider from JSON input
rad resource-provider create --from-file /path/to/input.json
`,
		Args: cobra.ExactArgs(0),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddFromFileFlagVar(cmd, &runner.ResourceProviderManifestFilePath)
	_ = cmd.MarkFlagRequired("from-file")
	_ = cmd.MarkFlagFilename("from-file", "yaml", "json")

	return cmd, runner
}

// Runner is the Runner implementation for the `rad resourceprovider create` command.
type Runner struct {
	ConnectionFactory connections.Factory
	ConfigHolder      *framework.ConfigHolder
	Output            output.Interface
	Format            string
	Workspace         *workspaces.Workspace

	ResourceProviderManifestFilePath string
	ResourceProvider                 *manifest.ResourceProvider
}

// NewRunner creates an instance of the runner for the `rad resourceprovider create` command.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resourceprovider create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
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

	return nil
}

// Run runs the `rad resourceprovider create` command.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	r.Output.LogInfo("Creating resource provider %s", r.ResourceProvider.Name)
	_, err = client.CreateOrUpdateResourceProvider(ctx, "local", r.ResourceProvider.Name, &v20231001preview.ResourceProviderResource{
		Location:   to.Ptr(v1.LocationGlobal),
		Properties: &v20231001preview.ResourceProviderProperties{},
	})
	if err != nil {
		return err
	}

	// The location resource contains references to all of the resource types and API versions that the resource provider supports.
	// We're instantiating the struct here so we can update it as we loop.
	locationResource := v20231001preview.LocationResource{
		Properties: &v20231001preview.LocationProperties{
			ResourceTypes: map[string]*v20231001preview.LocationResourceType{},
		},
	}

	for resourceTypeName, resourceType := range r.ResourceProvider.Types {
		r.Output.LogInfo("Creating resource type %s/%s", r.ResourceProvider.Name, resourceTypeName)
		_, err := client.CreateOrUpdateResourceType(ctx, "local", r.ResourceProvider.Name, resourceTypeName, &v20231001preview.ResourceTypeResource{
			Properties: &v20231001preview.ResourceTypeProperties{
				DefaultAPIVersion: resourceType.DefaultAPIVersion,
			},
		})
		if err != nil {
			return err
		}

		locationResourceType := &v20231001preview.LocationResourceType{
			APIVersions: map[string]map[string]any{},
		}

		for apiVersionName := range resourceType.APIVersions {
			r.Output.LogInfo("Creating API Version %s/%s@%s", r.ResourceProvider.Name, resourceTypeName, apiVersionName)
			_, err := client.CreateOrUpdateAPIVersion(ctx, "local", r.ResourceProvider.Name, resourceTypeName, apiVersionName, &v20231001preview.APIVersionResource{
				Properties: &v20231001preview.APIVersionProperties{},
			})
			if err != nil {
				return err
			}

			locationResourceType.APIVersions[apiVersionName] = map[string]any{}
		}

		locationResource.Properties.ResourceTypes[resourceTypeName] = locationResourceType
	}

	r.Output.LogInfo("Creating location %s/%s", r.ResourceProvider.Name, v1.LocationGlobal)
	_, err = client.CreateOrUpdateLocation(ctx, "local", r.ResourceProvider.Name, v1.LocationGlobal, &locationResource)
	if err != nil {
		return err
	}

	response, err := client.GetResourceProvider(ctx, "local", r.ResourceProvider.Name)
	if err != nil {
		return err
	}

	// Add a blank line before printing the result.
	r.Output.LogInfo("")

	err = r.Output.WriteFormatted(r.Format, response, common.GetResourceProviderTableFormat())
	if err != nil {
		return err
	}

	return nil
}
