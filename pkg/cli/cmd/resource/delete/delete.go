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

package delete

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/cli/cmd/commonflags"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmationWithoutApplicationOrEnvironment = "Are you sure you want to delete resource '%v' of type %v?"
	deleteConfirmationWithoutApplication              = "Are you sure you want to delete resource '%v' of type %v from environment '%v'?"
	deleteConfirmationWithApplication                 = "Are you sure you want to delete resource '%v' of type %v in application '%v' from environment '%v'?"
)

// NewCommand creates an instance of the command and runner for the `rad resource delete` command.
//

// NewCommand creates a new cobra command for deleting a Radius resource, with flags for output, workspace, resource group,
//
//	and confirmation. It returns the command and a Runner to execute the command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "delete [resourceType] [resourceName]",
		Short: "Delete a Radius resource",
		Long:  "Deletes a Radius resource with the given name",
		Example: `
sample list of resourceType: containers, gateways, daprPubSubBrokers, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores

# Delete a container named orders
rad resource delete containers orders`,
		Args: cobra.ExactArgs(2),
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddOutputFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddConfirmationFlag(cmd)

	return cmd, runner
}

// Runner is the runner implementation for the `rad resource delete` command.
type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Output            output.Interface
	Workspace         *workspaces.Workspace
	ResourceType      string
	ResourceName      string
	Format            string

	InputPrompter prompt.Interface
	Confirm       bool
}

// NewRunner creates a new instance of the `rad resource delete` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:      factory.GetConfigHolder(),
		ConnectionFactory: factory.GetConnectionFactory(),
		Output:            factory.GetOutput(),
		InputPrompter:     factory.GetPrompter(),
	}
}

// Validate runs validation for the `rad resource delete` command.
//

// Validate checks the workspace, scope, resource type and name, output format, and confirmation flag from the
// command line arguments and sets them in the Runner struct. It returns an error if any of these values are invalid.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
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

	resourceType, resourceName, err := cli.RequireResourceTypeAndName(args)
	if err != nil {
		return err
	}
	r.ResourceType = resourceType
	r.ResourceName = resourceName

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	yes, err := cmd.Flags().GetBool("yes")
	if err != nil {
		return err
	}
	r.Confirm = yes

	return nil
}

// Run runs the `rad resource delete` command.
//

// Run checks if the user has confirmed the deletion of the resource, and if so, attempts to delete the resource and
// logs the result. If an error occurs, it is returned.
func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	environmentID, applicationID, err := r.extractEnvironmentAndApplicationIDs(ctx, client)
	if clients.Is404Error(err) {
		r.Output.LogInfo("Resource '%s' of type '%s' does not exist or has already been deleted", r.ResourceName, r.ResourceType)
		return nil
	} else if err != nil {
		return err
	}

	// Prompt user to confirm deletion
	if !r.Confirm {
		var promptMessage string
		if applicationID.IsEmpty() && environmentID.IsEmpty() {
			promptMessage = fmt.Sprintf(deleteConfirmationWithoutApplicationOrEnvironment, r.ResourceName, r.ResourceType)
		} else if applicationID.IsEmpty() {
			promptMessage = fmt.Sprintf(deleteConfirmationWithoutApplication, r.ResourceName, r.ResourceType, environmentID.Name())
		} else {
			promptMessage = fmt.Sprintf(deleteConfirmationWithApplication, r.ResourceName, r.ResourceType, applicationID.Name(), environmentID.Name())
		}

		confirmed, err := prompt.YesOrNoPrompt(promptMessage, prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("resource %q of type %q NOT deleted", r.ResourceName, r.ResourceType)
			return nil
		}
	}

	deleted, err := client.DeleteResource(ctx, r.ResourceType, r.ResourceName)
	if err != nil {
		return err
	}

	if deleted {
		r.Output.LogInfo("Resource deleted")
	} else {
		r.Output.LogInfo("Resource '%s' of type '%s' does not exist or has already been deleted", r.ResourceName, r.ResourceType)
	}

	return nil
}

func (r *Runner) extractEnvironmentAndApplicationIDs(ctx context.Context, client clients.ApplicationsManagementClient) (environmentID resources.ID, applicationID resources.ID, err error) {
	resource, err := client.GetResource(ctx, r.ResourceType, r.ResourceName)
	if err != nil {
		return resources.ID{}, resources.ID{}, err
	}

	// Note: The following cases are all possible:
	//
	// 1. The resource has an environment and an application. (common case for a standard resource)
	// 2. The resource has an environment but no application. (possible case for a *shared* standard resource)
	// 3. The resource has an application but no environment. (common case for a *core* resource like a container)
	//		- In this case, the environment can be looked up through the application
	//		- See: https://github.com/radius-project/radius/issues/2928
	// 4. The resource has no environment or application. (eg: a Bicep deployment)
	if resource.Properties["environment"] != nil {
		environmentID, err = convertToResourceID(resource.Properties["environment"])
		if err != nil {
			return resources.ID{}, resources.ID{}, err
		}
	}

	if resource.Properties["application"] != nil {
		applicationID, err = convertToResourceID(resource.Properties["application"])
		if err != nil {
			return resources.ID{}, resources.ID{}, err
		}
	}

	// Detect case 4: (no environment or application)
	if environmentID.IsEmpty() && applicationID.IsEmpty() {
		return resources.ID{}, resources.ID{}, nil
	}

	// At this point we have the environment and application IDs **if** they were returned by
	// the API. That covers case 1 & 2. Now we need to handle case 3, by doing an additional
	// lookup.
	if !environmentID.IsEmpty() {
		return environmentID, applicationID, nil // Case 1 or Case 2
	}

	if applicationID.IsEmpty() {
		return resources.ID{}, resources.ID{}, nil
	}

	application, err := client.GetApplication(ctx, applicationID.String())
	if clients.Is404Error(err) {
		// Ignore 404s for this case, and just assume there is no application. The user is
		// likely just doing cleanup and we don't want to block them.
		return environmentID, resources.ID{}, nil
	} else if err != nil {
		return resources.ID{}, resources.ID{}, err
	}

	environmentID, err = resources.ParseResource(*application.Properties.Environment)
	if err != nil {
		return resources.ID{}, resources.ID{}, err
	}

	return environmentID, applicationID, nil
}

func convertToResourceID(value any) (resources.ID, error) {
	resourceIDRaw, ok := value.(string)
	if !ok {
		return resources.ID{}, fmt.Errorf("resource ID is not a string")
	}

	return resources.ParseResource(resourceIDRaw)
}
