// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
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
		Use:   "create [envType]",
		Short: "create workspace",
		Long:  "Show details of the specified Radius resource",
		Args:  cobra.MinimumNArgs(1),
		Example: `# create a kubernetes workspace with name 'myworkspace' and kuberentes context 'aks'
	rad workspace create kubernetes -w myworkspace --context aks`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	cmd.Flags().StringP("kubecontext", "", "", "the Kubernetes context to use, will use the default if unset")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
	Force             bool
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	if args[0] != "kubernetes" {
		return fmt.Errorf("currently we support only kubernetes")
	}

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	env, err := cmd.Flags().GetString("env")
	if err != nil {
		return err
	}

	workspaceName, err := cmd.Flags().GetString("workspace")
	if err != nil {
		return err
	}

	workspaceExists, err := cli.HasWorkspace(config, workspaceName)
	if err != nil {
		return err
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	if !force && workspaceExists {
		return fmt.Errorf("Workspace exists. Please specify --force to overwrite")
	}

	if workspaceExists {
		workspace, err := cli.GetWorkspace(config, workspaceName)
		if err != nil {
			return err
		}
		r.Workspace = workspace
	} else {
		r.Workspace = &workspaces.Workspace{}
	}

	err = cli.RequireKubeContext(cmd)
	if err != nil {
		return err
	}

	if group != "" {
		r.Workspace.Scope = "/planes/radius/local/resourceGroups/" + group
	}
	if env != "" {
		r.Workspace.Environment = r.Workspace.Scope + "/providers/applications.core/environments/" + env
	}

	if !strings.HasPrefix(r.Workspace.Environment, r.Workspace.Scope) {
		return fmt.Errorf("Please use an environment which is in the scope of this workspace")
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {

	step := output.BeginStep("Creating Workspace...")

	err := cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		workspace := r.Workspace
		name := strings.ToLower(workspace.Name)
		section.Default = name
		section.Items[name] = *workspace

		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current workspace", r.Workspace.Name)
	output.CompleteStep(step)

	return nil
}
