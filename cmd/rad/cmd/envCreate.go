// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	client_go "k8s.io/client-go/kubernetes"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/kubernetes"
)

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

	return nil

}
