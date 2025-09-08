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
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/objectformats"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/spf13/cobra"
)

const (
	// defaultPlaneName is the default plane name for Radius
	defaultPlaneName = "local"
	// listTimeout is the timeout for list operations
	listTimeout = 30 * time.Second
)

// listFilter encapsulates filter parameters for resource listing
type listFilter struct {
	resourceType  string
	groupName     string
	environmentID string
	applicationID string
	planeName     string
}

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
rad resource list Applications.Core/containers -a icecream-store

# list all resources of a specified type in a group
rad resource list Applications.Core/containers --group test-group
rad resource list Applications.Core/containers -g test-group

# list all resources in a group
rad resource list --group test-group
rad resource list -g test-group

# list all resources of a specified type in an environment
rad resource list Applications.Core/containers --environment test-env
rad resource list Applications.Core/containers -e test-env

# list all resources in an environment
rad resource list --environment test-env
rad resource list -e test-env

# list resources with multiple filters (group + application)
rad resource list Applications.Core/containers --group test-group --application icecream-store
rad resource list Applications.Core/containers -g test-group -a icecream-store

# list resources with multiple filters (environment + application)
rad resource list Applications.Core/containers --environment test-env --application icecream-store
rad resource list Applications.Core/containers -e test-env -a icecream-store

# list resources with multiple filters (group + environment)
rad resource list Applications.Core/containers --group test-group --environment test-env
rad resource list Applications.Core/containers -g test-group -e test-env

# list resources with all filters (group + environment + application)
rad resource list Applications.Core/containers --group test-group --environment test-env --application icecream-store
rad resource list Applications.Core/containers -g test-group -e test-env -a icecream-store
`,
		Args: cobra.MaximumNArgs(1),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource list` command.
type Runner struct {
	ConfigHolder              *framework.ConfigHolder
	UCPClientFactory          *v20231001preview.ClientFactory
	ConnectionFactory         connections.Factory
	Output                    output.Interface
	Workspace                 *workspaces.Workspace
	ApplicationName           string
	EnvironmentName           string
	GroupName                 string
	PlaneName                 string
	Format                    string
	ResourceType              string
	ResourceTypeSuffix        string
	ResourceProviderNamespace string
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
//

// Validate checks the command line args, workspace, scope, application name, resource type and output format, and
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

	// Extract plane name from workspace scope
	r.PlaneName = extractPlaneName(scope)

	// Read filter flags
	applicationName, err := cli.ReadApplicationName(cmd, *workspace)
	if err != nil {
		return err
	}
	r.ApplicationName = applicationName

	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}
	r.EnvironmentName = environmentName

	groupName, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}
	r.GroupName = groupName

	// Resource type is optional now
	if len(args) > 0 {
		r.ResourceProviderNamespace, r.ResourceTypeSuffix, err = cli.RequireFullyQualifiedResourceType(args)
		if err != nil {
			return err
		}
		r.ResourceType = r.ResourceProviderNamespace + "/" + r.ResourceTypeSuffix
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

// Run checks if an application name is provided and if so, checks if the application exists in the workspace, then
// lists all resources of the specified type in the application, and finally writes the resources to the output in the
// specified format. If no application name is provided, it lists all resources of the specified type. An error is
// returned if the application does not exist in the workspace.
func (r *Runner) Run(ctx context.Context) error {
	// Add timeout for list operations
	ctx, cancel := context.WithTimeout(ctx, listTimeout)
	defer cancel()
	// Initialize the client factory if it hasn't been set externally.
	// This allows for flexibility where a test UCPClientFactory can be set externally during testing.
	if r.UCPClientFactory == nil {
		clientFactory, err := cmd.InitializeClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.UCPClientFactory = clientFactory
	}

	// Validate resource type if provided
	if r.ResourceType != "" {
		_, err := common.GetResourceTypeDetails(ctx, r.ResourceProviderNamespace, r.ResourceTypeSuffix, r.UCPClientFactory)
		if err != nil {
			return err
		}
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// Convert names to IDs if needed
	environmentID := ""
	if r.EnvironmentName != "" {
		environmentID, err = r.resolveEnvironmentID(ctx, client, r.EnvironmentName)
		if err != nil {
			return err
		}
	}

	applicationID := ""
	if r.ApplicationName != "" {
		applicationID, err = r.resolveApplicationID(ctx, client, r.ApplicationName)
		if err != nil {
			return err
		}
	}

	// Build filter context
	filter := &listFilter{
		resourceType:  r.ResourceType,
		groupName:     r.GroupName,
		environmentID: environmentID,
		applicationID: applicationID,
		planeName:     r.PlaneName,
	}

	// Determine listing strategy based on filters
	resourceList, err := r.executeListStrategy(ctx, client, filter)
	if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetGenericResourceTableFormat())
}

// executeListStrategy determines the appropriate listing strategy based on the filters
func (r *Runner) executeListStrategy(ctx context.Context, client clients.ApplicationsManagementClient, filter *listFilter) ([]generated.GenericResource, error) {
	// Group-based queries take precedence
	if filter.groupName != "" {
		return r.listWithGroup(ctx, client, filter)
	}

	// Environment-based queries
	if filter.environmentID != "" {
		return r.listWithEnvironment(ctx, client, filter)
	}

	// Application-only queries
	if filter.applicationID != "" {
		return r.listWithApplication(ctx, client, filter)
	}

	// No filters
	if filter.resourceType != "" {
		return client.ListResourcesOfType(ctx, filter.resourceType)
	}

	return r.listAllResources(ctx, client)
}

// listWithGroup handles group-based resource listing
func (r *Runner) listWithGroup(ctx context.Context, client clients.ApplicationsManagementClient, filter *listFilter) ([]generated.GenericResource, error) {
	if filter.resourceType != "" {
		// Specific resource type
		if filter.environmentID != "" || filter.applicationID != "" {
			return client.ListResourcesOfTypeInResourceGroupFiltered(ctx, filter.planeName, filter.groupName,
				filter.resourceType, filter.environmentID, filter.applicationID)
		}
		return client.ListResourcesOfTypeInResourceGroup(ctx, filter.planeName, filter.groupName, filter.resourceType)
	}

	// All resource types
	if filter.environmentID != "" || filter.applicationID != "" {
		return client.ListResourcesInResourceGroupFiltered(ctx, filter.planeName, filter.groupName,
			filter.environmentID, filter.applicationID)
	}
	return client.ListResourcesInResourceGroup(ctx, filter.planeName, filter.groupName)
}

// listWithEnvironment handles environment-based resource listing
func (r *Runner) listWithEnvironment(ctx context.Context, client clients.ApplicationsManagementClient, filter *listFilter) ([]generated.GenericResource, error) {
	if filter.resourceType != "" {
		// Specific resource type
		if filter.applicationID != "" {
			return r.listResourcesWithEnvironmentAndApplication(ctx, client, filter.environmentID, filter.applicationID)
		}
		return client.ListResourcesOfTypeInEnvironment(ctx, filter.environmentID, filter.resourceType)
	}

	// All resource types
	if filter.applicationID != "" {
		allResources, err := client.ListResourcesInEnvironment(ctx, filter.environmentID)
		if err != nil {
			return nil, err
		}
		return r.filterByApplication(allResources, filter.applicationID), nil
	}
	return client.ListResourcesInEnvironment(ctx, filter.environmentID)
}

// listWithApplication handles application-based resource listing
func (r *Runner) listWithApplication(ctx context.Context, client clients.ApplicationsManagementClient, filter *listFilter) ([]generated.GenericResource, error) {
	if filter.resourceType != "" {
		return client.ListResourcesOfTypeInApplication(ctx, filter.applicationID, filter.resourceType)
	}
	return client.ListResourcesInApplication(ctx, filter.applicationID)
}

// resolveEnvironmentID converts an environment name to a fully qualified resource ID
func (r *Runner) resolveEnvironmentID(ctx context.Context, client clients.ApplicationsManagementClient, environmentName string) (string, error) {
	// If it's already an ID, return it
	if strings.HasPrefix(environmentName, "/") {
		return environmentName, nil
	}

	// Convert name to ID using workspace scope
	environmentID := fmt.Sprintf("%s/providers/Applications.Core/environments/%s", r.Workspace.Scope, environmentName)

	// Verify the environment exists
	_, err := client.GetEnvironment(ctx, environmentID)
	if clients.Is404Error(err) {
		return "", clierrors.Message("The environment %q could not be found in workspace %q. Make sure you specify the correct environment with '-e/--environment'.", environmentName, r.Workspace.Name)
	} else if err != nil {
		return "", err
	}

	return environmentID, nil
}

// resolveApplicationID converts an application name to a fully qualified resource ID
func (r *Runner) resolveApplicationID(ctx context.Context, client clients.ApplicationsManagementClient, applicationName string) (string, error) {
	// If it's already an ID, return it
	if strings.HasPrefix(applicationName, "/") {
		return applicationName, nil
	}

	// Convert name to ID using workspace scope
	applicationID := fmt.Sprintf("%s/providers/Applications.Core/applications/%s", r.Workspace.Scope, applicationName)

	// Verify the application exists
	_, err := client.GetApplication(ctx, applicationID)
	if clients.Is404Error(err) {
		return "", clierrors.Message("The application %q could not be found in workspace %q. Make sure you specify the correct application with '-a/--application'.", applicationName, r.Workspace.Name)
	} else if err != nil {
		return "", err
	}

	return applicationID, nil
}

// listAllResources lists all resources across all resource types
func (r *Runner) listAllResources(ctx context.Context, client clients.ApplicationsManagementClient) ([]generated.GenericResource, error) {
	// Get all resource types
	resourceTypes, err := client.ListAllResourceTypesNames(ctx, r.PlaneName)
	if err != nil {
		return nil, err
	}

	var allResources []generated.GenericResource

	for _, resourceType := range resourceTypes {
		resources, err := client.ListResourcesOfType(ctx, resourceType)
		if err != nil {
			// Continue processing other resource types to return partial results
			// This allows the command to show available resources even if some types fail
			r.Output.LogInfo("Warning: Failed to list resources of type %q: %v", resourceType, err)
			continue
		}
		allResources = append(allResources, resources...)
	}

	return allResources, nil
}

// listResourcesWithEnvironmentAndApplication lists resources filtered by both environment and application
func (r *Runner) listResourcesWithEnvironmentAndApplication(ctx context.Context, client clients.ApplicationsManagementClient, environmentID, applicationID string) ([]generated.GenericResource, error) {
	// Get resources in environment
	resources, err := client.ListResourcesOfTypeInEnvironment(ctx, environmentID, r.ResourceType)
	if err != nil {
		return nil, err
	}

	// Filter by application
	return r.filterByApplication(resources, applicationID), nil
}

// filterByApplication filters a list of resources by application ID
func (r *Runner) filterByApplication(resources []generated.GenericResource, applicationID string) []generated.GenericResource {
	var filtered []generated.GenericResource
	for _, resource := range resources {
		if appID, ok := resource.Properties["application"].(string); ok && strings.EqualFold(appID, applicationID) {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

// extractPlaneName extracts the plane name from a scope string
func extractPlaneName(scope string) string {
	scopeParts := strings.Split(scope, "/")
	for i, part := range scopeParts {
		if part == "radius" && i+1 < len(scopeParts) {
			return scopeParts[i+1]
		}
	}
	return defaultPlaneName
}
