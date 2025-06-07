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

package list

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad resource list` command.
//

// NewCommand creates a new Cobra command and a Runner to list resources of a specified type in an application or the
// default environment, and adds flags for application name, resource group, output and workspace.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "list [resourceType]",
		Short: "Lists resources",
		Long:  "List all resources of specified type",
		Example: `
sample list of resourceType: Applications.Core/containers, Applications.Core/gateways, Applications.Dapr/daprPubSubBrokers, Applications.Core/extenders, Applications.Datastores/mongoDatabases, Applications.Messaging/rabbitMQMessageQueues, Applications.Datastores/redisCaches, Applications.Datastores/sqlDatabases, Applications.Dapr/daprStateStores, Applications.Dapr/daprSecretStores

# list all resources of a specified type in the default environment
rad resource list Applications.Core/containers
rad resource list Applications.Core/gateways

# list all resources of a specified type in an application
rad resource list Applications.Core/containers --application icecream-store

# list all resources of a specified type in an application (shorthand flag)
rad resource list Applications.Core/containers -a icecream-store

# list all resources of a specified type in an environment
rad resource list Applications.Core/containers --environment dev-env

# list all resources of a specified type in an environment (shorthand flag)
rad resource list Applications.Core/containers -e dev-env

# list all resources in an environment (no resource type specified)
rad resource list --environment dev-env

# list all resources in the default environment
rad resource list
`,
		Args: func(cmd *cobra.Command, args []string) error {
			// If environment flag is provided, args are optional
			if cmd.Flags().Changed("environment") || cmd.Flags().Changed("e") {
				if len(args) > 1 {
					return fmt.Errorf("accepts at most 1 arg when environment flag is provided, received %d", len(args))
				}
				return nil
			}

			// Otherwise, exactly 1 argument is required
			if len(args) != 1 {
				return fmt.Errorf("accepts 1 arg(s), received %d", len(args))
			}
			return nil
		},
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource list` command.
type Runner struct {
	ConfigHolder              *framework.ConfigHolder
	ConnectionFactory         connections.Factory
	Output                    output.Interface
	Workspace                 *workspaces.Workspace
	ApplicationName           string
	EnvironmentName           string
	Format                    string
	ResourceType              string
	ResourceTypeSuffix        string
	ResourceProviderNameSpace string
}

// NewRunner creates a new instance of the `rad resource list` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
	}
}

// Validate runs validation for the `rad resource list` command.
//	// Validate checks the command line args, workspace, scope, application name, resource type and output format, and
// returns an error if any of these are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	// Validate command line args and
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config, r.ConfigHolder.DirectoryConfig)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	// Only set application name if explicitly provided via flag
	applicationFlag, err := cmd.Flags().GetString("application")
	if err != nil {
		return err
	}
	if applicationFlag != "" {
		r.ApplicationName = applicationFlag
	}

	// Only set environment name if explicitly provided via flag
	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	if environmentName != "" {
		r.EnvironmentName = environmentName
	}

	// If we're listing all resources in an environment (no resource type specified),
	// we don't need to validate the resource type
	if len(args) > 0 {
		r.ResourceProviderNameSpace, r.ResourceTypeSuffix, err = cli.RequireFullyQualifiedResourceType(args)
		if err != nil {
			return err
		}
		r.ResourceType = r.ResourceProviderNameSpace + "/" + r.ResourceTypeSuffix
	}

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

// Run runs the `rad resource list` command.
//

// Run lists resources based on provided filters (application name, environment name, resource type).
// Resources can be filtered by:
// 1. Both application and environment (intersection of resources in both)
// 2. Only application (all resources in the application)
// 3. Only environment (all resources in the environment)
// 4. Only resource type (all resources of that type)
// The command requires at least one filter parameter to be provided.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// If neither application name nor environment name is provided,
	// and no resource type is specified, we can't list all resources
	if r.ApplicationName == "" && r.EnvironmentName == "" && r.ResourceType == "" {
		return clierrors.Message("Please specify a resource type, application name, or environment name")
	}

	// If a resource type is specified, validate it
	if r.ResourceType != "" {
		_, err := common.GetResourceTypeDetails(ctx, r.ResourceProviderNameSpace, r.ResourceTypeSuffix, client)
		if err != nil {
			return err
		}
	}

	var resourceList []generated.GenericResource

	// Handle different filter combinations
	switch {
	case r.ApplicationName != "" && r.EnvironmentName != "":
		// Filter resources by both application and environment
		resourceList, err = r.getResourcesInApplicationAndEnvironment(ctx, client)
	case r.ApplicationName != "":
		// Filter resources by application only
		resourceList, err = r.getResourcesInApplication(ctx, client)
	case r.EnvironmentName != "":
		// Filter resources by environment only
		if r.ResourceType != "" {
			// Use helper method for consistent handling
			resourceList, err = r.getResourcesInEnvironment(ctx, client)
		} else {
			resourceList, err = client.ListResourcesInEnvironment(ctx, r.EnvironmentName)
		}
	default:
		// Filter resources by resource type only
		resources, err := client.ListResourcesOfType(ctx, r.ResourceType)
		if err != nil {
			return err
		}
		resourceList = resources
	}

	if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetGenericResourceTableFormat())
}

// getResourcesInApplication retrieves resources in the specified application,
// optionally filtered by resource type.
func (r *Runner) getResourcesInApplication(ctx context.Context, client clients.ApplicationsManagementClient) ([]generated.GenericResource, error) {
	// Verify application exists
	_, err := client.GetApplication(ctx, r.ApplicationName)
	if err != nil {
		if clients.Is404Error(err) {
			return nil, clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", r.ApplicationName, r.Workspace.Name)
		}
		return nil, err
	}

	// Get resources filtered by application and optionally by resource type
	if r.ResourceType != "" {
		return client.ListResourcesOfTypeInApplication(ctx, r.ApplicationName, r.ResourceType)
	}
	return client.ListResourcesInApplication(ctx, r.ApplicationName)
}

// getResourcesInEnvironment retrieves resources in the specified environment,
// optionally filtered by resource type.
func (r *Runner) getResourcesInEnvironment(ctx context.Context, client clients.ApplicationsManagementClient) ([]generated.GenericResource, error) {
	// Verify environment exists
	_, err := client.GetEnvironment(ctx, r.EnvironmentName)
	if err != nil {
		if clients.Is404Error(err) {
			return nil, clierrors.Message("The environment %q could not be found in workspace %q. Make sure you specify the correct environment with '-e/--environment'.", r.EnvironmentName, r.Workspace.Name)
		}
		return nil, err
	}

	var resources []generated.GenericResource
	// Get resources filtered by environment and optionally by resource type
	if r.ResourceType != "" {
		resources, err = client.ListResourcesOfTypeInEnvironment(ctx, r.EnvironmentName, r.ResourceType)
		if err != nil {
			return nil, err
		}
	} else {
		resources, err = client.ListResourcesInEnvironment(ctx, r.EnvironmentName)
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}

// getResourcesInApplicationAndEnvironment retrieves resources that belong to both
// the specified application and environment.
func (r *Runner) getResourcesInApplicationAndEnvironment(ctx context.Context, client clients.ApplicationsManagementClient) ([]generated.GenericResource, error) {
	// Verify application exists
	_, err := client.GetApplication(ctx, r.ApplicationName)
	if err != nil {
		if clients.Is404Error(err) {
			return nil, clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", r.ApplicationName, r.Workspace.Name)
		}
		return nil, err
	}

	// Verify environment exists
	_, err = client.GetEnvironment(ctx, r.EnvironmentName)
	if err != nil {
		if clients.Is404Error(err) {
			return nil, clierrors.Message("The environment %q could not be found in workspace %q. Make sure you specify the correct environment with '-e/--environment'.", r.EnvironmentName, r.Workspace.Name)
		}
		return nil, err
	}

	// Get resources in application (filtered by resource type if specified)
	var appResources []generated.GenericResource
	if r.ResourceType != "" {
		appResources, err = client.ListResourcesOfTypeInApplication(ctx, r.ApplicationName, r.ResourceType)
	} else {
		appResources, err = client.ListResourcesInApplication(ctx, r.ApplicationName)
	}
	if err != nil {
		return nil, err
	}

	// Get resources in environment (filtered by resource type if specified)
	var envResources []generated.GenericResource
	if r.ResourceType != "" {
		envResources, err = client.ListResourcesOfTypeInEnvironment(ctx, r.EnvironmentName, r.ResourceType)
	} else {
		envResources, err = client.ListResourcesInEnvironment(ctx, r.EnvironmentName)
	}
	if err != nil {
		return nil, err
	}

	// Find intersection: resources that belong to both application and environment
	return findCommonResources(appResources, envResources), nil
}

// findCommonResources returns a list of resources that exist in both resource lists,
// comparing them by resource ID.
func findCommonResources(appResources, envResources []generated.GenericResource) []generated.GenericResource {
	result := []generated.GenericResource{}

	// Create a map of environment resource IDs for faster lookup
	envResourceMap := make(map[string]bool)
	for _, resource := range envResources {
		if resource.ID != nil {
			envResourceMap[*resource.ID] = true
		}
	}

	// Filter application resources that are also in the environment
	for _, resource := range appResources {
		if resource.ID != nil && envResourceMap[*resource.ID] {
			result = append(result, resource)
		}
	}
	return result
}