// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/project-radius/radius/pkg/cli/setup"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [envType]",
		Short: "create workspace",
		Long:  "Show details of the specified Radius resource",
		Example: `
	# create a kubernetes workspace with name 'myworkspace' and kuberentes context 'aks'
	rad workspace create kubernetes -w myworkspace --context aks
	`,
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP("workspace", "w", "", "The workspace name")
	cmd.Flags().BoolP("interactive", "i", false, "Collect values for required command arguments through command line interface prompts")
	cmd.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	cmd.Flags().StringP("kubecontext", "", "", "the Kubernetes context to use, will use the default if unset")
	cmd.Flags().StringP("group", "g", "", "the radius resource group to use, this resource group must be already created with rad group or rad init")
	cmd.Flags().StringP("env", "e", "", "the environment resource to use, this environment must be already created with rad env or rad init")

	return cmd, runner
}

type Runner struct {
	ConfigHolder      *framework.ConfigHolder
	ConnectionFactory connections.Factory
	Workspace         *workspaces.Workspace
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory: factory.GetConnectionFactory(),
		ConfigHolder:      factory.GetConfigHolder(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	kubecontext, err := cmd.Flags().GetString("kubecontext")
	if err != nil {
		return err
	}

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	env, err := cmd.Flags().GetString("env")
	if err != nil {
		return err
	}

	name, err := cli.ReadWorkspaceNameArgs(cmd, args)
	if err != nil {
		return err
	}

	if name != "" {
		// Name was specified at the command line - validate uniqueness.
		existing, err := cli.HasWorkspace(config, name)
		if err != nil {
			return err
		}

		if existing && !force {
			return fmt.Errorf("the workspace %q already exists. Specify '--force' to overwrite", name)
		}
	} else if interactive {

		name, err = prompt.Text(
			"Enter the name to use for the workspace:",
			prompt.MatchAll(prompt.ResourceName, setup.ValidateWorkspaceUniqueness(config, force)))
		if err != nil {
			return err
		}
	} else {
		return fmt.Errorf("the workspace name is required")
	}

	// We validate the context and make sure we actually store a named context (not the empty string)
	kubeconfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return err
	}

	if kubecontext == "" && kubeconfig.CurrentContext == "" {
		return errors.New("the kubeconfig has no current context")
	} else if kubecontext == "" {
		kubecontext = kubeconfig.CurrentContext
	} else {
		_, ok := kubeconfig.Contexts[kubecontext]
		if !ok {
			return fmt.Errorf("the kubeconfig does not contain a context called %q", kubecontext)
		}
	}
	/*

		workspace = &workspaces.Workspace{
			Connection: map[string]interface{}{
				"kind":    "kubernetes",
				"context": contextName,
			},
			Scope:    id,
			Registry: registry,
			Name:     workspaceName,
		}
	*/
	r.Workspace = &workspaces.Workspace{
		Connection: map[string]interface{}{
			"kind":    "kubernetes",
			"context": kubecontext,
		},

		Name: name,
	}
	if group != "" {
		r.Workspace.Scope = "/planes/radius/local/resourceGroups/" + group
		if env != "" {
			r.Workspace.Environment = r.Workspace.Scope + "/providers/applications.core/environments/" + env
		}
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
