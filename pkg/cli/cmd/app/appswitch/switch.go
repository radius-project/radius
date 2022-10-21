// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package appswitch

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)
	cmd := &cobra.Command{
		Use:     "switch",
		Short:   "Switch the default RAD application",
		Long:    "Switches the default RAD application",
		Args:    cobra.ExactArgs(1),
		Example: `rad app switch newApplication`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddApplicationNameFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Format              string
	Workspace           *workspaces.Workspace
	ApplicationName     string
	ConnectionFactory   connections.Factory
	AppManagementClient clients.ApplicationsManagementClient
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		AppManagementClient: factory.GetAppManagementClient(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.ApplicationName, err = cli.ReadApplicationNameArgs(cmd, args)
	if err != nil {
		return err
	}

	// HEY YOU: Keep the logic below here in sync with `rad env switch``
	if strings.EqualFold(r.Workspace.DefaultApplication, r.ApplicationName) {
		output.LogInfo("Default application is already set to %v", r.ApplicationName)
		return nil
	}

	r.AppManagementClient, err = r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
	if err != nil {
		return err
	}

	//ignore applicationresource as we only check for existence of application
	_, err = r.AppManagementClient.ShowApplication(cmd.Context(), r.ApplicationName)
	if cli.Is404ErrorForAzureError(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("Unable to switch applications as the requested application %s does not exist.\n", r.ApplicationName)}
	}

	if workspace.DefaultApplication == "" {
		output.LogInfo("Switching default application to %v", r.ApplicationName)
	} else {
		output.LogInfo("Switching default application from %v to %v", workspace.DefaultApplication, r.ApplicationName)
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	err := cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		r.Workspace.DefaultApplication = r.ApplicationName
		section.Items[strings.ToLower(r.Workspace.Name)] = *r.Workspace
		return nil
	})
	if err != nil {
		return err
	}

	return nil
}
