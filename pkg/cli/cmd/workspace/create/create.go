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
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/spf13/cobra"
)

// NewCommand creates an instance of the command and runner for the `rad workspace create` command.
func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create [workspaceType] [workspaceName]",
		Short: "Create a workspace",
		Long: `Create a workspace.
		
Available workspaceTypes: kubernetes

Workspaces allow you to manage multiple Radius platforms and environments using a local configuration file. 

You can easily define and switch between workspaces to deploy and manage applications across local, test, and production environments.`,
		Args: ValidateArgs(),
		Example: `
# Create a workspace with name 'myworkspace' and kubernetes context 'aks'
rad workspace create kubernetes myworkspace --context aks
# Create a workspace with name of current kubernetes context in current kubernetes context
rad workspace create kubernetes`,
		RunE: framework.RunCommand(runner),
	}

	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddEnvironmentNameFlag(cmd)
	cmd.Flags().BoolP("force", "f", false, "Overwrite existing workspace if present")
	cmd.Flags().StringP("context", "c", "", "the Kubernetes context to use, will use the default if unset")

	return cmd, runner
}

// Runner is the runner implementation for the `rad workspace create` command.
type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	ConnectionFactory   connections.Factory
	Workspace           *workspaces.Workspace
	Force               bool
	ConfigFileInterface framework.ConfigFileInterface
	Output              output.Interface
	HelmInterface       helm.Interface
	KubernetesInterface kubernetes.Interface
}

// NewRunner creates a new instance of the `rad workspace create` runner.
func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigHolder:        factory.GetConfigHolder(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		Output:              factory.GetOutput(),
		HelmInterface:       factory.GetHelmInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
	}
}

// Validate runs validation for the `rad workspace create` command.
func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspaceName, err := cli.ReadWorkspaceNameSecondArg(cmd, args)
	if err != nil {
		return err
	}

	kubeContextList, err := r.KubernetesInterface.GetKubeContext()
	if err != nil {
		return &cli.FriendlyError{Message: "Failed to read kube config"}
	}
	context, err := cli.RequireKubeContext(cmd, kubeContextList.CurrentContext)
	if err != nil {
		return err
	}

	_, ok := kubeContextList.Contexts[context]
	if !ok {
		return fmt.Errorf("the kubeconfig does not contain a context called %q", context)
	}

	if workspaceName == "" {
		workspaceName = context
	}

	installed, err := r.HelmInterface.CheckRadiusInstall(context)
	if !installed || (err != nil) {
		return fmt.Errorf("unable to create workspace %q. Radius control plane not installed on target platform. Run 'rad install' and try again", workspaceName)
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
		return fmt.Errorf("workspace exists. please specify --force to overwrite")
	}

	if workspaceExists {
		workspace, err := cli.GetWorkspace(config, workspaceName)
		if err != nil {
			return err
		}
		r.Workspace = workspace
	} else {
		r.Workspace = &workspaces.Workspace{}
		r.Workspace.Name = workspaceName
	}
	r.Workspace.Connection = map[string]any{}
	r.Workspace.Connection["context"] = context
	r.Workspace.Connection["kind"] = args[0]

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	env, err := cmd.Flags().GetString("environment")
	if err != nil {
		return err
	}

	var client clients.ApplicationsManagementClient
	if group != "" {
		r.Workspace.Scope = "/planes/radius/local/resourceGroups/" + group

		client, err = r.ConnectionFactory.CreateApplicationsManagementClient(cmd.Context(), *r.Workspace)
		if err != nil {
			return err
		}
		_, err := client.ShowUCPGroup(cmd.Context(), "radius", "local", group)
		if err != nil {
			return &cli.FriendlyError{Message: fmt.Sprintf("group %q does not exist. Run `rad env create` try again \n", r.Workspace.Scope)}
		}

		//we want to make sure we dont have a workspace which has environment in a different scope from workspace's scope
		if r.Workspace.Environment != "" && !strings.HasPrefix(r.Workspace.Environment, r.Workspace.Scope) && env == "" {
			return fmt.Errorf("workspace is currently using an environment which is in different scope. use -e to specify an environment which is in the scope of this workspace")
		}
	}
	if env != "" {
		if r.Workspace.Scope == "" {
			return fmt.Errorf("cannot set environment for workspace with empty scope. use -g to set a scope")
		}
		r.Workspace.Environment = r.Workspace.Scope + "/providers/applications.core/environments/" + env

		_, err = client.GetEnvDetails(cmd.Context(), env)
		if err != nil {
			return &cli.FriendlyError{Message: fmt.Sprintf("environment %q does not exist. Run `rad env create` try again \n", r.Workspace.Environment)}
		}
	}

	return nil
}

// Run runs the `rad workspace create` command.
func (r *Runner) Run(ctx context.Context) error {

	r.Output.LogInfo("creating workspace...")
	err := r.ConfigFileInterface.EditWorkspaces(ctx, r.ConfigHolder.Config, r.Workspace, []interface{}{})
	if err != nil {
		return err
	}
	output.LogInfo("Set %q as current workspace", r.Workspace.Name)

	return nil
}
