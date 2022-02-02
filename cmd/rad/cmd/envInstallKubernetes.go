// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	_ "embed"

	"github.com/project-radius/radius/pkg/cli"
	"github.com/project-radius/radius/pkg/cli/environments"
	"github.com/project-radius/radius/pkg/cli/helm"
	"github.com/project-radius/radius/pkg/cli/output"
	"github.com/project-radius/radius/pkg/cli/prompt"
	"github.com/spf13/cobra"
)

var envInstallKubernetesCmd = &cobra.Command{
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

		chartPath, err := cmd.Flags().GetString("chart")
		if err != nil {
			return err
		}

		image, err := cmd.Flags().GetString("image")
		if err != nil {
			return err
		}

		tag, err := cmd.Flags().GetString("tag")
		if err != nil {
			return err
		}

		if interactive {
			namespace, err = prompt.Text("Enter a namespace name:", prompt.EmptyValidator)
			if err != nil {
				return err
			}
		}

		step := output.BeginStep("Installing Radius...")

		client, runtimeClient, contextName, err := createKubernetesClients("")
		if err != nil {
			return err
		}

		err = installRadius(cmd.Context(), client, runtimeClient, namespace, chartPath, image, tag)
		if err != nil {
			return err
		}

		err = installGateway(cmd.Context(), runtimeClient, helm.HAProxyOptions{UseHostNetwork: true})
		if err != nil {
			return err
		}

		output.CompleteStep(step)

		config := ConfigFromContext(cmd.Context())

		env, err := cli.ReadEnvironmentSection(config)
		if err != nil {
			return err
		}

		if environmentName == "" {
			environmentName = contextName
		}

		env.Items[environmentName] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   contextName,
			"namespace": namespace,
		}

		output.LogInfo("Using environment: %v", environmentName)
		env.Default = environmentName
		cli.UpdateEnvironmentSection(config, env)

		err = cli.SaveConfig(config)
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInstallCmd.AddCommand(envInstallKubernetesCmd)
	envInstallKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInstallKubernetesCmd.Flags().StringP("namespace", "n", "default", "The namespace to use for the environment")
	envInstallKubernetesCmd.Flags().StringP("chart", "c", "", "Specify a file path to a helm chart to install radius from")
	envInstallKubernetesCmd.Flags().String("image", "", "Specify the radius controller image to use")
	envInstallKubernetesCmd.Flags().String("tag", "", "Specify the radius controller tag to use")
}
