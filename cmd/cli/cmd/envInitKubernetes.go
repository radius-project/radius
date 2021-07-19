// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"
	"errors"
	"fmt"

	"github.com/Azure/radius/pkg/helm"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/kubernetes"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/spf13/cobra"
)

var envInitKubernetesCmd = &cobra.Command{
	Use:   "kubernetes",
	Short: "Initializes a kubernetes environment",
	Long:  `Initializes a kubernetes environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		environmentName, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		interactive, err := cmd.Flags().GetBool("interactive")
		if err != nil {
			return err
		}

		namespace, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return err
		}

		version, err := cmd.Flags().GetString("version")
		if err != nil {
			return err
		}

		if interactive {
			namespace, err = prompt.Text("Enter a namespace name:", prompt.EmptyValidator)
			if err != nil {
				return err
			}
		}

		k8sconfig, err := kubernetes.ReadKubeConfig()
		if err != nil {
			return err
		}

		if k8sconfig.CurrentContext == "" {
			return errors.New("no kubernetes context is set")
		}

		context := k8sconfig.Contexts[k8sconfig.CurrentContext]
		if context == nil {
			return fmt.Errorf("kubernetes context '%s' could not be found", k8sconfig.CurrentContext)
		}

		step := logger.BeginStep("Installing Radius...")

		client, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
		if err != nil {
			return err
		}

		// Do note: the namespace passed in to rad env init kubernetes
		// doesn't match the namespace where we deploy the controller to.
		// The controller and other resources are all deployed to the
		// 'radius-system' namespace. The namespace passed in will be
		// where pods/services/deployments will be put for rad deploy.
		err = kubernetes.CreateNamespace(cmd.Context(), client, helm.RadiusSystemNamespace)
		if err != nil {
			return err
		}

		err = helm.ApplyRadiusHelmChart(version)
		if err != nil {
			return err
		}

		logger.CompleteStep(step)

		config := ConfigFromContext(cmd.Context())

		env, err := rad.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		if environmentName == "" {
			environmentName = k8sconfig.CurrentContext
		}

		env.Items[environmentName] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   k8sconfig.CurrentContext,
			"namespace": namespace,
		}

		logger.LogInfo("using environment %v", environmentName)
		env.Default = environmentName
		rad.UpdateEnvironmentSection(config, env)

		err = rad.SaveConfig(config)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInitKubernetesCmd.Flags().StringP("version", "v", "", "The version of the Radius runtime to install, for example: 0.3.0, defaults to the latest version")
}
