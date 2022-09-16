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

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	coreRpApps "github.com/project-radius/radius/pkg/corerp/api/v20220315privatepreview"
	"github.com/spf13/cobra"

	client_go "k8s.io/client-go/kubernetes"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/connections"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/workspaces"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func NewCommand(factory framework.Factory, k8sGoClient client_go.Interface) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory, k8sGoClient)

	cmd := &cobra.Command{
		Use:     "create environment",
		Short:   "create environment",
		Long:    "Create a new Radius environment",
		Example: ``,
		// Args: cobra.ExactArgs(),
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "Specify the resource group to create environment in")
	cmd.Flags().StringP("namespace", "n", "", "Specify the environment namespace")

	return cmd, runner
}

type Runner struct {
	ConfigHolder     *framework.ConfigHolder
	Output           output.Interface
	Workspace        *workspaces.Workspace
	EnvironmentName  string
	UCPResourceGroup string
	Namespace        string
	K8sGoClient      client_go.Interface
	KubeContext      string
}

func NewRunner(factory framework.Factory, k8sGoClient client_go.Interface) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		K8sGoClient:  k8sGoClient,
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config

	// TODO: use require workspace
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}
	// if workspace.Name == "" {
	// 	section, err := cli.ReadWorkspaceSection(config)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	workspace.Name = section.Default
	// }

	// workspace, err := cli.GetWorkspace(config, workspace.Name)
	// if err != nil {
	// 	return err
	// }
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
	fmt.Println(r.Namespace)

	r.UCPResourceGroup, err = cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	// TODO: check if resource group exists
	err = kubernetes.EnsureNamespace(cmd.Context(), r.K8sGoClient, r.Namespace)
	if err != nil {
		return err
	}

	kubeconfig, err := kubernetes.ReadKubeConfig()
	if err != nil {
		return err
	}

	if kubeconfig.CurrentContext == "" {
		return errors.New("the kubeconfig has no current context")
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
	step := output.BeginStep("Creating Environment...")

	scopeId, err := resources.Parse(r.Workspace.Scope)
	if err != nil {
		return err
	}

	environmentID, err := createEnvironmentResource(ctx, r.KubeContext, scopeId.FindScope(resources.ResourceGroupsSegment), r.EnvironmentName, r.Namespace)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(ctx, r.ConfigHolder.Config, func(section *cli.WorkspaceSection) error {
		ws := section.Items[strings.ToLower(r.Workspace.Name)]
		ws.Environment = environmentID
		section.Items[strings.ToLower(r.Workspace.Name)] = ws
		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current environment for workspace %q", r.EnvironmentName, r.Workspace.Name)

	output.CompleteStep(step)
	return nil
}

func createEnvironmentResource(ctx context.Context, kubeCtxName, resourceGroupName, environmentName string, namespace string) (string, error) {
	baseURL, transporter, err := kubernetes.CreateAPIServerTransporter(kubeCtxName, "")
	if err != nil {
		return "", fmt.Errorf("failed to create environment client: %w", err)
	}

	loc := "global"
	id := "self"

	toCreate := coreRpApps.EnvironmentResource{
		Location: &loc,
		Properties: &coreRpApps.EnvironmentProperties{
			Compute: &coreRpApps.KubernetesCompute{
				Kind:       to.Ptr(coreRpApps.EnvironmentComputeKindKubernetes),
				ResourceID: &id,
				Namespace:  to.Ptr(namespace),
			},
		},
	}

	rootScope := fmt.Sprintf("planes/radius/local/resourceGroups/%s", resourceGroupName)

	envClient, err := coreRpApps.NewEnvironmentsClient(rootScope, &aztoken.AnonymousCredential{}, connections.GetClientOptions(baseURL, transporter))
	if err != nil {
		return "", fmt.Errorf("failed to create environment client: %w", err)
	}

	resp, err := envClient.CreateOrUpdate(ctx, environmentName, toCreate, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create Applications.Core/environments resource: %w", err)
	}

	return *resp.ID, nil
}
