// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	"embed"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/kubernetes"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/Azure/radius/pkg/version"
	"github.com/Azure/radius/test/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//go:embed Chart
var chartFolder embed.FS

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
		err = install(cmd.Context(), KubernetesInitConfig{
			Namespace: namespace,
			Version:   version.Version(),
		})
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
}

// runKubectlApply runs a kubectl CLI command with stdout and stderr buffered for logging when there is an error.
func runKubectlApply(ctx context.Context, content []byte) error {
	var executableName string
	var executableArgs []string
	if runtime.GOOS == "windows" {
		// Use shell on windows since az is a script not an executable
		executableName = fmt.Sprintf("%s\\system32\\cmd.exe", os.Getenv("windir"))
		executableArgs = append(executableArgs, "/c", "kubectl")
	} else {
		executableName = "kubectl"
	}

	// kubectl can accept a file via stdin via passing '-f -'.
	// Ex: cat pod.json | kubectl apply -f - would pass pod.json to kubectl apply.
	executableArgs = append(executableArgs, "apply", "-f", "-")
	c := exec.CommandContext(ctx, executableName, executableArgs...)

	buf := bytes.Buffer{}
	stdin, err := c.StdinPipe()
	if err != nil {
		return err
	}

	radiusChart, err := radiusChart(config.Version, helmConf)
	if err != nil {
		return err
	}

	version, err := getVersion(config.Version)
	if err != nil {
		return err
	}

	err = applyCRDs(fmt.Sprintf("v%s", version))
	if err != nil {
		return err
	}

	go func() {
		// ignore errors from copy failing
		_, _ = io.Copy(&buf, stdout)
	}()

	go func() {
		// ignore errors from copy failing
		_, _ = io.Copy(&buf, stderr)
	}()

	values, err := chartValues(config)
	if err != nil {
		return err
	}

	if _, err = installClient.Run(radiusChart, values); err != nil {
		return err
	}

	return err
}
