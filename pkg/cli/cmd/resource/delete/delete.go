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
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

const (
	deleteConfirmation = "Are you sure you want to delete resource %v of type %v?"
)

// NewCommand creates an instance of the command and runner for the `rad resource delete` command.
//
// # Function Explanation
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
		sample list of resourceType: containers, gateways, httpRoutes, daprPubSubBrokers, extenders, mongoDatabases, rabbitMQMessageQueues, redisCaches, sqlDatabases, daprStateStores, daprSecretStores
		
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
// # Function Explanation
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
// # Function Explanation
//
// Run checks if the user has confirmed the deletion of the resource, and if so, attempts to delete the resource and
// logs the result. If an error occurs, it is returned.
func (r *Runner) Run(ctx context.Context) error {
	// Prompt user to confirm deletion
	if !r.Confirm {
		confirmed, err := prompt.YesOrNoPrompt(fmt.Sprintf(deleteConfirmation, r.ResourceName, r.ResourceType), prompt.ConfirmNo, r.InputPrompter)
		if err != nil {
			return err
		}
		if !confirmed {
			r.Output.LogInfo("resource %q of type %q NOT deleted", r.ResourceName, r.ResourceType)
			return nil
		}
	}

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	var respFromCtx *http.Response
	ctxWithResp := runtime.WithCaptureResponse(ctx, &respFromCtx)

	deleted, err := client.DeleteResource(ctxWithResp, r.ResourceType, r.ResourceName)
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
