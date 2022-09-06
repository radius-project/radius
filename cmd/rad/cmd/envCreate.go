// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/framework"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/cli/objectformats"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

func NewCommand(factory framework.Factory) (*cobra.Command, framework.Runner) {
	runner := NewRunner(factory)

	cmd := &cobra.Command{
		Use:     "",
		Short:   "",
		Long:    "",
		Example: ``,
		Args:    cobra.ExactArgs(),
		RunE:    framework.RunCommand(runner),
	}

	outputDescription := fmt.Sprintf("output format (supported formats are %s)", strings.Join(output.SupportedFormats(), ", "))
	// Define your flags here
	cmd.Flags().StringP("flagName", "k (flag's shorthand notation like w for workspace)", "", "What does the flag ask for")

	return cmd, runner
}

type Runner struct {
	ConfigHolder *framework.ConfigHolder
	Output       output.Interface
}

func NewRunner(factory framework.Factory) *Runner {
	return &Runner{
		ConfigHolder: factory.GetConfigHolder(),
		Output:       factory.GetOutput(),
	}
}

func (r *Runner) Validate(cmd *cobra.Command, args []string) error {
	workspace, err := cli.RequireWorkspace(cmd, r.ConfigHolder.Config)
	if err != nil {
		return err
	}
	r.Workspace = workspace

	format, err := cli.RequireOutput(cmd)
	if err != nil {
		return err
	}
	r.Format = format

	return nil
}

func (r *Runner) Run(cmd *cobra.Command, args []string) error {
	err = r.Output.Write(r.Format, resourceDetails, objectformats.GetResourceTableFormat())
	if err != nil {
		return err
	}

	return nil
}

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

}
