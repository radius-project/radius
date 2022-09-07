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
	runtime_client "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli"
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
		Example: ``,
		// Args: cobra.ExactArgs(),
		RunE: framework.RunCommand(runner),
	}

	cmd.Flags().StringP("group", "g", "", "Specify the resource group to create environment in")
	cmd.Flags().StringP("namespace", "n", "", "Specify the environment namespace")

	// outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	// Define your flags here
	// cmd.Flags().StringP("flagName", "k (flag's shorthand notation like w for workspace)", "", "What does the flag ask for")

	return cmd, runner
}

type Runner struct {
	ConfigHolder     *framework.ConfigHolder
	Output           output.Interface
	Workspace        *workspaces.Workspace
	EnvironmentName  string
	UCPResourceGroup string
	Namespace        string
	K8SClient        client_go.Interface
	// add k8s client here
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
		// add k8s client here
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	config := r.ConfigHolder.Config
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}
	r.EnvironmentName = environmentName

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if namespace == "" {
		r.Namespace = environmentName
	}

	group, err := cmd.Flags().GetString("group")
	if err != nil {
		return err
	}

	var k8sGoClient client_go.Interface
	k8sGoClient, _, contextName, err := createKubernetesClients("")
	fmt.Println("context name: ", contextName)
	if err != nil {
		return err
	}

	err = kubernetes.EnsureNamespace(cmd.Context(), k8sGoClient, namespace)
	if err != nil {
		return err
	}

	//ctx := r.Context()
	// response, err := h.ucp.ResourceGroups.List(ctx, h.db, h.getRelativePath(r.URL.Path))

	kubeconfig, err := kubernetes.ReadKubeConfig()
	// fmt.Println("kubeconfig: %s", kubeconfig)

	if err != nil {
		return err
	}

	// kubecontext :=
	if kubeconfig.CurrentContext == "" {
		return errors.New("the kubeconfig has no current context")
	}

	kubecontext := kubeconfig.CurrentContext
	isRadiusInstalled, err := helm.CheckRadiusInstall(kubecontext)
	if err != nil {
		return err
	}
	// fmt.Println("rad install: %s", isRadiusInstalled)
	if !isRadiusInstalled {
		return fmt.Errorf("unable to reach workspace %s. Check your workspace configuration and try again", namespace)
	}

	// fmt.Println(resGroup)
	fmt.Println(environmentName)

	r.Workspace = &workspaces.Workspace{
		Connection: map[string]interface{}{
			"kind":    "kubernetes",
			"context": kubecontext,
		},

		Name: workspace.Name,
	}

	// r.Environment = &environments.Environment{}

	if group != "" {
		r.Workspace.Scope = "/planes/radius/local/resourceGroups/" + group
		if environmentName != "" {
			r.Workspace.Environment = r.Workspace.Scope + "/providers/applications.core/environments/" + r.EnvironmentName
		}
	}

	return nil
}

func (r *Runner) Run(ctx context.Context) error {
	step := output.BeginStep("Creating Environment...")
	fmt.Println(r.Workspace)

	scopeId, err := resources.Parse(r.Workspace.Scope)
	if err != nil {
		return err
	}

	//fmt.Println(scopeId)
	//fmt.Println(scopeId.FindScope(resources.ResourceGroupsSegment))

	fmt.Println(r.Workspace.Connection["context"].(string))
	environmentID, err := createEnvironmentResource(ctx, r.Workspace.Connection["context"].(string), scopeId.FindScope(resources.ResourceGroupsSegment), r.EnvironmentName, r.Namespace)
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

func createKubernetesClients(contextName string) (client_go.Interface, runtime_client.Client, string, error) {
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

/*
var envCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create environment",
	Long:  `Create the specified Radius environment`,
	RunE:  createEnvResource,
}

func init() {
	envCmd.AddCommand(envCreateCmd)

	envCreateCmd.Flags().StringP("resourcegroup", "g", "", "Specify the resource group to create environment in")
	envCreateCmd.Flags().StringP("namespace", "n", "", "Specify the environment namespace")
}

func createEnvResource(cmd *cobra.Command, args []string) error {
	config := ConfigFromContext(cmd.Context())
	workspace, err := cli.RequireWorkspace(cmd, config)
	if err != nil {
		return err
	}

	environmentName, err := cli.RequireEnvironmentNameArgs(cmd, args, *workspace)
	if err != nil {
		return err
	}

	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	} else if namespace == "" {
		namespace = environmentName
	}

	var k8sGoClient client_go.Interface

	k8sGoClient, _, contextName, err := createKubernetesClients("")
	fmt.Println("context name: %s", contextName)
	if err != nil {
		return err
	}

	err = kubernetes.EnsureNamespace(cmd.Context(), k8sGoClient, namespace)
	if err != nil {
		return err
	}

	resGroup, err := cmd.Flags().GetString("resourcegroup")
	if err != nil {
		return err
	}

	//ctx := r.Context()
	// response, err := h.ucp.ResourceGroups.List(ctx, h.db, h.getRelativePath(r.URL.Path))

	kubeconfig, err := kubernetes.ReadKubeConfig()
	fmt.Println("kubeconfig: %s", kubeconfig)

	if err != nil {
		return err
	}

	// kubecontext :=
	if kubeconfig.CurrentContext == "" {
		return errors.New("the kubeconfig has no current context")
	}

	kubecontext := kubeconfig.CurrentContext
	isRadiusInstalled, err := helm.CheckRadiusInstall(kubecontext)
	if err != nil {
		return err
	}
	fmt.Println("rad install: %s", isRadiusInstalled)
	if !isRadiusInstalled {
		return fmt.Errorf("unable to reach workspace %s. Check your workspace configuration and try again", namespace)
	}

	fmt.Println(resGroup)
	fmt.Println(environmentName)

	step := output.BeginStep("Creating Environment...")
	fmt.Println(workspace.Scope)

	scopeId, err := resources.Parse(workspace.Scope)
	if err != nil {
		return err
	}

	//fmt.Println(scopeId)
	//fmt.Println(scopeId.FindScope(resources.ResourceGroupsSegment))

	environmentID, err := createEnvironmentResource(cmd.Context(), contextName, scopeId.FindScope(resources.ResourceGroupsSegment), environmentName, namespace)
	if err != nil {
		return err
	}

	err = cli.EditWorkspaces(cmd.Context(), config, func(section *cli.WorkspaceSection) error {
		ws := section.Items[strings.ToLower(workspace.Name)]
		ws.Environment = environmentID
		section.Items[strings.ToLower(workspace.Name)] = ws
		return nil
	})
	if err != nil {
		return err
	}

	output.LogInfo("Set %q as current environment for workspace %q", environmentName, workspace.Name)

	output.CompleteStep(step)
	return nil

}*/
