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

package preview

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	generated "github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/radius-project/radius/pkg/cli/cmd"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

const (
	msgApplicationDeletedPreview  = "Application deleted"
	msgDeletingApplicationPreview = "Deleting application %s...\n"
	msgDeletingResources          = "Deleting %d resource(s) associated with application %s...\n"
	bicepWarning                  = "'%v' is a Bicep filename or path and not the name of a Radius Application. Specify the name of a valid application and try again"
)

// NewCommand creates an instance of the command and runner for the `rad app delete --preview` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete Radius Application (preview)",
		Long:  `Delete application and its associated resources using the Radius.Core preview API surface.`,
		Args:  cobra.MaximumNArgs(1),
		Example: `
# Delete current application
rad app delete --preview

# Delete current application and bypass confirmation prompt
rad app delete --yes --preview

# Delete specified application
rad app delete my-app --preview
`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)
	commonflags.AddForceFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the preview `rad app delete` command.
type Runner struct {
	ConfigHolder            *framework.ConfigHolder
	Output                  output.Interface
	InputPrompter           prompt.Interface
	ConnectionFactory       connections.Factory
	Workspace               *workspaces.Workspace
	RadiusCoreClientFactory *corerpv20250801.ClientFactory

	Confirm         bool
	Force           bool
	ApplicationName string
}

// NewRunner creates a new instance of the preview delete runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
		ConnectionFactory: factory.GetConnectionFactory(),
	}
}

// Validate runs validation for the preview delete command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	scope, err := cli.RequireScope(cmd, *r.Workspace)
	if err != nil {
		return err
	}
	r.Workspace.Scope = scope

	r.ApplicationName, err = cli.RequireApplicationArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	if strings.HasSuffix(r.ApplicationName, ".bicep") {
		return clierrors.Message(bicepWarning, r.ApplicationName)
	}

	r.Confirm, err = cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}

	r.Force, err = cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	return nil
}

// Run executes the preview delete command logic.
//
// This discovers resources owned by the application using the management client's
// resource enumeration (ownership-based via properties.application), deletes them
// in parallel, then deletes the application via the Radius.Core preview API.
func (r *Runner) Run(ctx context.Context) error {
	if r.RadiusCoreClientFactory == nil {
		factory, err := cmd.InitializeRadiusCoreClientFactory(ctx, r.Workspace, r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.RadiusCoreClientFactory = factory
	}

	appClient := r.RadiusCoreClientFactory.NewApplicationsClient()

	// Check if the application exists
	_, err := appClient.Get(ctx, r.ApplicationName, &corerpv20250801.ApplicationsClientGetOptions{})
	if clients.Is404Error(err) {
		r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
		return nil
	} else if err != nil {
		return err
	}

	if !r.Confirm {
		promptMsg := fmt.Sprintf("Are you sure you want to delete application '%s'?", r.ApplicationName)
		confirmed, err := prompt.YesOrNoPrompt(promptMsg, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("Application %q NOT deleted", r.ApplicationName)
			return nil
		}
	}

	if r.Force {
		r.Output.LogInfo("WARNING: Force deleting an application. Resources in non-terminal states may leave orphaned external resources that require manual cleanup.")
	}

	// Use the management client to discover and delete owned resources.
	// This uses ownership-based filtering (properties.application matches our app ID)
	// rather than GetGraph which returns a connectivity graph that may include shared resources.
	managementClient, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	// Build the fully qualified Radius.Core application ID for ownership matching
	applicationID := r.Workspace.Scope + "/providers/Radius.Core/applications/" + r.ApplicationName

	resourcesList, err := listResourcesOwnedByApplication(ctx, managementClient, applicationID)
	if err != nil && !clients.Is404Error(err) {
		return err
	}

	// Delete associated resources in parallel
	if len(resourcesList) > 0 {
		r.Output.LogInfo(msgDeletingResources, len(resourcesList), r.ApplicationName)

		g, groupCtx := errgroup.WithContext(ctx)
		for _, resource := range resourcesList {
			if resource.ID != nil && resource.Type != nil {
				// Log before launching the goroutine; output.Interface implementations
				// (including the MockOutput used in tests) are not guaranteed to be
				// thread-safe, and ordering the log here keeps output deterministic.
				r.Output.LogInfo("  Deleting %s...", *resource.ID)
				resourceType := *resource.Type
				resourceID := *resource.ID
				g.Go(func() error {
					_, err := managementClient.DeleteResource(groupCtx, resourceType, resourceID, r.Force)
					if err != nil && !clients.Is404Error(err) {
						return err
					}
					return nil
				})
			}
		}

		if err := g.Wait(); err != nil {
			return clierrors.Message("Failed to delete resources for application '%s': %v", r.ApplicationName, err)
		}
	}

	// Delete the application itself via the preview API.
	// Re-check for 404 in case the app was concurrently deleted during resource cleanup.
	r.Output.LogInfo(msgDeletingApplicationPreview, r.ApplicationName)

	_, err = appClient.Delete(ctx, r.ApplicationName, &corerpv20250801.ApplicationsClientDeleteOptions{})
	if clients.Is404Error(err) {
		r.Output.LogInfo("Application '%s' does not exist or has already been deleted.", r.ApplicationName)
		return nil
	} else if err != nil {
		return err
	}

	r.Output.LogInfo(msgApplicationDeletedPreview)
	return nil
}

// listResourcesOwnedByApplication lists resources whose properties.application field
// matches the given application ID. This is an ownership-based query that only returns
// resources explicitly owned by the application, unlike GetGraph which returns a
// connectivity graph that may include shared/environment resources.
func listResourcesOwnedByApplication(ctx context.Context, client clients.ApplicationsManagementClient, applicationID string) ([]generated.GenericResource, error) {
	resourceTypesList, err := client.ListAllResourceTypesNames(ctx, "local")
	if err != nil {
		return nil, err
	}

	var results []generated.GenericResource
	for _, resourceType := range resourceTypesList {
		resources, err := client.ListResourcesOfType(ctx, resourceType)
		if err != nil {
			return nil, err
		}

		for _, resource := range resources {
			if isResourceOwnedByApplication(resource, applicationID) {
				results = append(results, resource)
			}
		}
	}

	return results, nil
}

// isResourceOwnedByApplication checks if a resource's properties.application field
// matches the given application ID (case-insensitive).
func isResourceOwnedByApplication(resource generated.GenericResource, applicationID string) bool {
	obj, found := resource.Properties["application"]
	if !found {
		return false
	}

	associatedAppID, ok := obj.(string)
	if !ok || associatedAppID == "" {
		return false
	}

	return strings.EqualFold(associatedAppID, applicationID)
}
