// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package envCreate

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	client_go "k8s.io/client-go/kubernetes"
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/cmd/commonflags"
	"github.com/project-radius/radius/pkg/cli/configFile"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "create environment",
		Short:   "create environment",
		Long:    "Create a new Radius environment",
		Args:    cobra.MinimumNArgs(1),
		Example: `rad env create -e myenv`,
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
	KubeContext         string
	ConnectionFactory   connections.Factory
	ScopeID             resources.ID
	ConfigFileInterface configFile.Interface
	KubernetesInterface kubernetes.Interface
}

func NewRunner(factory framework.Factory) *Runner {
	k8sGoClient, _, _, err := CreateKubernetesClients("")
	if err != nil {
		fmt.Println(err)
	}

	return &Runner{
		ConfigHolder:        factory.GetConfigHolder(),
		Output:              factory.GetOutput(),
		K8sGoClient:         k8sGoClient,
		ConnectionFactory:   factory.GetConnectionFactory(),
		ConfigFileInterface: factory.GetConfigFileInterface(),
		KubernetesInterface: factory.GetKubernetesInterface(),
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
		r.ScopeID = scopeId
	}

	kubeconfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return err
	}

	if kubeconfig.CurrentContext == "" {
		return fmt.Errorf("the kubeconfig has no current context")
	}

	r.KubeContext = kubeconfig.CurrentContext
	isRadiusInstalled, err := helm.CheckRadiusInstall(r.KubeContext)
	if err != nil {
		return err
	}

	if !isRadiusInstalled {
		return fmt.Errorf("unable to reach workspace %s. Check your workspace configuration and try again", r.Workspace.Name)
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	r.Output.LogInfo("Creating Environment...")

	client, err := r.ConnectionFactory.CreateApplicationsManagementClient(ctx, *r.Workspace)
	if err != nil {
		return err
	}

	err = r.KubernetesInterface.EnsureNamespace(ctx, r.K8sGoClient, r.Namespace)
	if err != nil {
		return err
	}

	_, err = client.GetUCPGroup(ctx, "radius", "local", r.UCPResourceGroup)
	if err != nil {
		return err
	}

	isEnvCreated, err := client.CreateEnvironment(ctx, r.EnvironmentName, "global", r.Namespace, "Kubernetes", "")
	if err != nil || !isEnvCreated {
		return err
	}

	err = r.ConfigFileInterface.EditWorkspaces(ctx, r.ConfigHolder.ConfigFilePath, r.Workspace.Name, r.EnvironmentName)
	// err = cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
	// 	ws := section.Items[strings.ToLower(r.Workspace.Name)]
	// 	envId := r.ScopeID.Append(resources.TypeSegment{Type: "Applications.Core/environments", Name: r.EnvironmentName})
	// 	ws.Environment = envId.String()
	// 	section.Items[strings.ToLower(r.Workspace.Name)] = ws
	// 	return nil
	// })
	if err != nil {
		return err
	}

	r.Output.LogInfo("Set %q as current environment for workspace %q", r.EnvironmentName, r.Workspace.Name)

	return nil
}

func CreateKubernetesClients(contextName string) (client_go.Interface, runtime_client.Client, string, error) {
	k8sConfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return nil, nil, "", err
	}

	if contextName == "" && k8sConfig.CurrentContext == "" {
		return nil, nil, "", errors.New("no kubernetes context is set")
	} else if contextName == "" {
		contextName = k8sConfig.CurrentContext
	}

	context := k8sConfig.Contexts[contextName]
	if context == nil {
		return nil, nil, "", fmt.Errorf("kubernetes context '%s' could not be found", contextName)
	}

	client, _, err := kubernetes.CreateTypedClient(contextName)
	if err != nil {
		return nil, nil, "", err
	}

	runtimeClient, err := kubernetes.CreateRuntimeClient(contextName, kubernetes.Scheme)
	if err != nil {
		return nil, nil, "", err
	}

	return client, runtimeClient, contextName, nil
}
