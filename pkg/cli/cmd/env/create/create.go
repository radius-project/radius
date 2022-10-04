// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package create

import (
	"context"
	"fmt"

	corerp "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/cmd/env/namespace"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new environment",
		Long: `Create a new Radius environment
Radius environments are prepared "landing zones" for Radius applications.
Applications deployed to an environment will inherit the container runtime, configuration, and other settings from the environment.`,
		Args:    cobra.MinimumNArgs(1),
		Example: `rad env create myenv`,
		RunE:    framework.RunCommand(runner),
	}

	commonflags.AddEnvironmentNameFlag(cmd)
	commonflags.AddWorkspaceFlag(cmd)
	commonflags.AddResourceGroupFlag(cmd)
	commonflags.AddNamespaceFlag(cmd)

	return cmd, runner
}

type Runner struct {
	ConfigHolder        *framework.ConfigHolder
	Output              output.Interface
	Workspace           *workspaces.Workspace
	EnvironmentName     string
	UCPResourceGroup    string
	Namespace           string
	K8sGoClient         client_go.Interface
	ConnectionFactory   connections.Factory
	ConfigFileInterface framework.ConfigFileInterface
	KubernetesInterface kubernetes.Interface
	NamespaceInterface  namespace.Interface
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
		NamespaceInterface:  factory.GetNamespaceInterface(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	r.EnvironmentName, err = cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	r.Namespace, err = cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if r.Namespace == "" {
		r.Namespace = r.EnvironmentName
	}

	r.UCPResourceGroup, err = cmd.Flags().GetString("group")
	if err != nil {
		return err
	}
	if r.UCPResourceGroup == "" {
		scopeId, err := resources.Parse(r.Workspace.Scope)
		if err != nil {
			return err
		}
		r.UCPResourceGroup = scopeId.FindScope(resources.ResourceGroupsSegment)
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	r.K8sGoClient, _, _, err = kubernetes.CreateKubernetesClients("")
	if err != nil {
		return err
	}
	err = r.NamespaceInterface.ValidateNamespace(ctx, r.K8sGoClient, r.Namespace)
	if err != nil {
		return err
	}

	_, err = client.ShowUCPGroup(ctx, "radius", "local", r.UCPResourceGroup)
	if cli.Is404ErrorForAzureError(err) {
		return &cli.FriendlyError{Message: fmt.Sprintf("Resource group %q could not be found.", r.UCPResourceGroup)}
	} else if err != nil {
		return err
	}

	r.Output.LogInfo("Creating Environment...")

	isEnvCreated, err := client.CreateEnvironment(ctx, r.EnvironmentName, "global", r.Namespace, "Kubernetes", "", map[string]*corerp.EnvironmentRecipeProperties{})
	if err != nil || !isEnvCreated {
		return err
	}

	r.Output.LogInfo("Successfully created environment %q in workspace %q", r.EnvironmentName, r.Workspace.Name)

	return nil
}
