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

	"github.com/spf13/cobra"

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
	"github.com/radius-project/radius/pkg/ucp/resources"
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
		Long: `List resources with flexible filtering options.
When no parameters are provided, lists all resources in the workspace's active resource group.
The active resource group can be changed using 'rad group switch'.
When resource type is specified without filters, lists all resources of that type in the workspace.
Resources can be filtered by application, environment, resource group, or any combination of these filters.`,
		Example: `
sample list of resourceType: Applications.Core/containers, Applications.Core/gateways, Applications.Dapr/daprPubSubBrokers, Applications.Core/extenders, Applications.Datastores/mongoDatabases, Applications.Messaging/rabbitMQMessageQueues, Applications.Datastores/redisCaches, Applications.Datastores/sqlDatabases, Applications.Dapr/daprStateStores, Applications.Dapr/daprSecretStores

# list all resources in the workspace's active resource group (default behavior)
rad resource list

# list all resources of a specified type in the workspace
rad resource list Applications.Core/containers
rad resource list Applications.Core/gateways

# list all applications (top-level resources)
rad resource list Applications.Core/applications

# list all environments (top-level resources)
rad resource list Applications.Core/environments

# list all resources in a specific group (overrides default)
rad resource list --group test-group

# list all resources of a specified type in an application
rad resource list Applications.Core/containers --application icecream-store
rad resource list Applications.Core/containers -a icecream-store

# list all resources of a specified type in a group
rad resource list Applications.Core/containers --group test-group
rad resource list Applications.Core/containers -g test-group

# list applications in a specific group
rad resource list Applications.Core/applications --group prod-group

# list environments in a specific group
rad resource list Applications.Core/environments --group prod-group

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

// ResourceNotFoundError represents a resource that couldn't be found
type ResourceNotFoundError struct {
	ResourceType string
	Name         string
	Workspace    string
}

func (e ResourceNotFoundError) Error() string {
	return clierrors.Message("The %s %q could not be found in workspace %q. Make sure you specify the correct %s with the appropriate flag.",
		e.ResourceType, e.Name, e.Workspace, e.ResourceType).Error()
}

// Runner is the runner implementation for the `rad resource list` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	UCPClientFactory  *v20231001preview.ClientFactory
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	Format            string

	// Filter Options
	Filter struct {
		ApplicationName string
		EnvironmentName string
		GroupName       string
		ResourceType    string
	}

	PlaneName                 string
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
	applicationName, err := cmd.Flags().GetString("application")
	if err != nil {
		return err
	}
	r.Filter.ApplicationName = applicationName

	environmentName, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}
	r.Filter.EnvironmentName = environmentName

	groupName, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}
	r.Filter.GroupName = groupName

	// Resource type is optional now
	if len(args) > 0 {
		r.ResourceProviderNamespace, r.ResourceTypeSuffix, err = cli.RequireFullyQualifiedResourceType(args)
		if err != nil {
			return err
		}
		r.Filter.ResourceType = r.ResourceProviderNamespace + "/" + r.ResourceTypeSuffix
	} else {
		// When no resource type is specified and no filters provided,
		// default to listing all resources in the workspace's active resource group
		if r.Filter.GroupName == "" && r.Filter.EnvironmentName == "" && r.Filter.ApplicationName == "" {
			// Extract group name from workspace scope (already parsed above)
			scopeID, err := resources.ParseScope(r.Workspace.Scope)
			if err != nil {
				return err
			}
			r.Filter.GroupName = scopeID.Name()

			// Log this default behavior so users understand what's happening
			r.Output.LogInfo("No filters specified. Listing all resources in workspace's active resource group %q", r.Filter.GroupName)
		}
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

	// Initialize dependencies
	if err := r.initializeDependencies(ctx); err != nil {
		return err
	}

	// Create management client
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// Build filter with resolved IDs
	filter, err := r.buildFilter(ctx, client)
	if err != nil {
		return err
	}

	// Execute list operation
	resourceList, err := r.executeListStrategy(ctx, client, filter)
	if err != nil {
		return err
	}

	return r.Output.WriteFormatted(r.Format, resourceList, objectformats.GetGenericResourceTableFormat())
}

// initializeDependencies initializes the client factory and validates resource type
func (r *Runner) initializeDependencies(ctx context.Context) error {
	if r.UCPClientFactory == nil {
		clientFactory, err := cmd.InitializeClientFactory(ctx, r.Workspace)
		if err != nil {
			return err
		}
		r.UCPClientFactory = clientFactory
	}

	// Validate resource type if provided
	if r.Filter.ResourceType != "" {
		_, err := common.GetResourceTypeDetails(ctx, r.ResourceProviderNamespace, r.ResourceTypeSuffix, r.UCPClientFactory)
		if err != nil {
			return err
		}
	}

	return nil
}

// resourceBelongsToGroup checks if a resource ID belongs to the specified resource group
func (r *Runner) resourceBelongsToGroup(resourceID string, groupName string) bool {
	if resourceID == "" || groupName == "" {
		return false
	}

	parsed, err := resources.Parse(resourceID)
	if err != nil {
		return false
	}

	segments := parsed.ScopeSegments()
	for _, seg := range segments {
		if strings.EqualFold(seg.Type, "resourceGroups") {
			return strings.EqualFold(seg.Name, groupName)
		}
	}

	return false
}

// buildFilter creates a filter with resolved resource IDs
func (r *Runner) buildFilter(ctx context.Context, client clients.ApplicationsManagementClient) (*listFilter, error) {
	// Step 1: Validate group exists if specified
	if r.Filter.GroupName != "" {
		_, err := client.GetResourceGroup(ctx, r.PlaneName, r.Filter.GroupName)
		if clients.Is404Error(err) {
			return nil, ResourceNotFoundError{
				ResourceType: "resource group",
				Name:         r.Filter.GroupName,
				Workspace:    r.Workspace.Name,
			}
		} else if err != nil {
			return nil, err
		}
	}

	// Step 2: Resolve and validate environment
	environmentID, err := func() (string, error) {
		var (
			name     string                                                                              = r.Filter.EnvironmentName
			resolver func(context.Context, clients.ApplicationsManagementClient, string) (string, error) = r.resolveEnvironmentID
		)
		return r.resolveResourceID(ctx, client, name, resolver)
	}()
	if err != nil {
		return nil, err
	}

	// Validate environment belongs to group if both are specified
	if r.Filter.GroupName != "" && environmentID != "" {
		if !r.resourceBelongsToGroup(environmentID, r.Filter.GroupName) {
			return nil, clierrors.Message("Environment %q not found in resource group %q.", r.Filter.EnvironmentName, r.Filter.GroupName)
		}
	}

	// Step 3: Resolve and validate application
	applicationID, err := func() (string, error) {
		var (
			name     string                                                                              = r.Filter.ApplicationName
			resolver func(context.Context, clients.ApplicationsManagementClient, string) (string, error) = r.resolveApplicationID
		)
		return r.resolveResourceID(ctx, client, name, resolver)
	}()
	if err != nil {
		return nil, err
	}

	// Validate application belongs to group if both are specified
	if r.Filter.GroupName != "" && applicationID != "" {
		if !r.resourceBelongsToGroup(applicationID, r.Filter.GroupName) {
			return nil, clierrors.Message("Application %q not found in resource group %q.", r.Filter.ApplicationName, r.Filter.GroupName)
		}
	}

	// Validate application belongs to environment if both are specified
	if environmentID != "" && applicationID != "" {
		app, err := client.GetApplication(ctx, applicationID)
		if err != nil {
			return nil, err
		}

		if app.Properties != nil && app.Properties.Environment != nil {
			if !strings.EqualFold(*app.Properties.Environment, environmentID) {
				return nil, clierrors.Message("Application %q does not belong to environment %q.", r.Filter.ApplicationName, r.Filter.EnvironmentName)
			}
		}
	}

	return &listFilter{
		resourceType:  r.Filter.ResourceType,
		groupName:     r.Filter.GroupName,
		environmentID: environmentID,
		applicationID: applicationID,
		planeName:     r.PlaneName,
	}, nil
}

// resolveResourceID is a generic helper to resolve resource names to IDs
func (r *Runner) resolveResourceID(ctx context.Context, client clients.ApplicationsManagementClient, name string, resolver func(context.Context, clients.ApplicationsManagementClient, string) (string, error)) (string, error) {
	if name == "" {
		return "", nil
	}
	return resolver(ctx, client, name)
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
			return r.listResourcesWithEnvironmentAndApplication(ctx, client, filter.environmentID, filter.applicationID, filter.resourceType)
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
	// If it's already an ID, validate group match if --group is set
	if strings.HasPrefix(environmentName, "/") {
		if r.Filter.GroupName != "" {
			parsed, err := resources.Parse(environmentName)
			if err == nil {
				segments := parsed.ScopeSegments()
				idGroup := ""
				for _, seg := range segments {
					if strings.EqualFold(seg.Type, "resourceGroups") {
						idGroup = seg.Name
						break
					}
				}
				if idGroup != "" && idGroup != r.Filter.GroupName {
					return "", clierrors.Message("The provided environment ID targets resource group %q but --group is set to %q.", idGroup, r.Filter.GroupName)
				}
			}
		}
		return environmentName, nil
	}

	// Use --group if set, otherwise workspace scope
	scope := r.Workspace.Scope
	if r.Filter.GroupName != "" {
		scope = fmt.Sprintf("/planes/radius/%s/resourceGroups/%s", r.PlaneName, r.Filter.GroupName)
	}
	environmentID := fmt.Sprintf("%s/providers/Applications.Core/environments/%s", scope, environmentName)

	_, err := client.GetEnvironment(ctx, environmentID)
	if clients.Is404Error(err) {
		return "", ResourceNotFoundError{
			ResourceType: "environment",
			Name:         environmentName,
			Workspace:    r.Workspace.Name,
		}
	} else if err != nil {
		return "", err
	}

	return environmentID, nil
}

// resolveApplicationID converts an application name to a fully qualified resource ID
func (r *Runner) resolveApplicationID(ctx context.Context, client clients.ApplicationsManagementClient, applicationName string) (string, error) {
	// If it's already an ID, validate group match if --group is set
	if strings.HasPrefix(applicationName, "/") {
		if r.Filter.GroupName != "" {
			parsed, err := resources.Parse(applicationName)
			if err == nil {
				segments := parsed.ScopeSegments()
				idGroup := ""
				for _, seg := range segments {
					if strings.EqualFold(seg.Type, "resourceGroups") {
						idGroup = seg.Name
						break
					}
				}
				if idGroup != "" && idGroup != r.Filter.GroupName {
					return "", clierrors.Message("The provided application ID targets resource group %q but --group is set to %q.", idGroup, r.Filter.GroupName)
				}
			}
		}
		return applicationName, nil
	}

	// Use --group if set, otherwise workspace scope
	scope := r.Workspace.Scope
	if r.Filter.GroupName != "" {
		scope = fmt.Sprintf("/planes/radius/%s/resourceGroups/%s", r.PlaneName, r.Filter.GroupName)
	}
	applicationID := fmt.Sprintf("%s/providers/Applications.Core/applications/%s", scope, applicationName)

	_, err := client.GetApplication(ctx, applicationID)
	if clients.Is404Error(err) {
		return "", ResourceNotFoundError{
			ResourceType: "application",
			Name:         applicationName,
			Workspace:    r.Workspace.Name,
		}
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
func (r *Runner) listResourcesWithEnvironmentAndApplication(ctx context.Context, client clients.ApplicationsManagementClient, environmentID, applicationID string, resourceType string) ([]generated.GenericResource, error) {
	// Get resources in environment
	resources, err := client.ListResourcesOfTypeInEnvironment(ctx, environmentID, resourceType)
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
