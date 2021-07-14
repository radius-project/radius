// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/pkg/rad/environments"
	"github.com/Azure/radius/pkg/rad/kubernetes"
	"github.com/Azure/radius/pkg/rad/logger"
	"github.com/Azure/radius/pkg/rad/prompt"
	"github.com/spf13/cobra"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	k8s "k8s.io/client-go/kubernetes"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	radiusSystemNamespace = "radius-system"
	helmDriverSecret      = "secret"
)

func createNamespace(ctx context.Context, client *k8s.Clientset, namespace string) error {
	namespaceApply := applycorev1.Namespace(namespace)

	// Use Apply instead of Create to avoid failures on a namespace already existing.
	_, err := client.CoreV1().Namespaces().Apply(ctx, namespaceApply, metav1.ApplyOptions{FieldManager: "rad"})
	if err != nil {
		return err
	}
	return nil
}

func helmConfig(namespace string) (*helm.Configuration, error) {
	hc := helm.Configuration{}
	flags := &genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}

	// helmDriver is "secret" to make the backend storage driver
	// use kubernetes secrets.
	err := hc.Init(flags, namespace, helmDriverSecret, debugLogf)
	return &hc, err
}

func createTempDir() (string, error) {
	dir, err := ioutil.TempDir("", radiusReleaseName)
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %s", err)
	}
	return dir, nil
}

func locateChartFile(dirPath string) (string, error) {
	files, err := ioutil.ReadDir(dirPath)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", errors.New("radius helm chart not found")
	}

	if len(files) > 1 {
		return "", errors.New("unexpected files found when downloading helm chart")
	}

	return filepath.Join(dirPath, files[0].Name()), nil
}

func radiusChart(version string, config *helm.Configuration) (*chart.Chart, error) {
	pull := helm.NewPull()
	pull.RepoURL = radiusHelmRepo
	pull.Settings = &cli.EnvSettings{}

	// If version isn't set, it will use the latest version.
	if version != "" {
		pull.Version = version
	}

	dir, err := createTempDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	pull.DestDir = dir

	_, err = pull.Run(radiusReleaseName)
	if err != nil {
		return nil, err
	}

	chartPath, err := locateChartFile(dir)
	if err != nil {
		return nil, err
	}
	return loader.Load(chartPath)
}

func debugLogf(format string, v ...interface{}) {
}

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

		err = createNamespace(cmd.Context(), client, radiusSystemNamespace)
		if err != nil {
			return err
		}

		helmConf, err := helmConfig(radiusSystemNamespace)
		if err != nil {
			return err
		}

		radiusChart, err := radiusChart(version, helmConf)
		if err != nil {
			return err
		}

		// https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you
		// TODO: Apply CRDs because Helm doesn't upgrade CRDs for you.
		// https://github.com/Azure/radius/issues/712
		// We need the CRDs to be public to do this (or consider unpacking the chart
		// for the CRDs)

		histClient := helm.NewHistory(helmConf)
		histClient.Max = 1 // Only need to check if at least 1 exists

		// See: https://github.com/helm/helm/blob/281380f31ccb8eb0c86c84daf8bcbbd2f82dc820/cmd/helm/upgrade.go#L99
		// The upgrade client's install option doesn't seem to work, so we have to check the history of releases manually
		// and invoke the install client.
		_, err = histClient.Run(radiusReleaseName)

		if err == driver.ErrReleaseNotFound {
			logger.LogInfo("Installing new Radius Kubernetes environment to namespace: %s", radiusSystemNamespace)

			installClient := helm.NewInstall(helmConf)
			installClient.ReleaseName = radiusReleaseName
			installClient.Namespace = radiusSystemNamespace
			if _, err = installClient.Run(radiusChart, radiusChart.Values); err != nil {
				return err
			}
		} else if err == nil {
			logger.LogInfo("Found existing Radius Kubernetes environment, upgrading")
			upgradeClient := helm.NewUpgrade(helmConf)
			upgradeClient.Namespace = radiusSystemNamespace

			if _, err = upgradeClient.Run(radiusReleaseName, radiusChart, radiusChart.Values); err != nil {
				return err
			}
		} else {
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
	envInitKubernetesCmd.Flags().StringP("version", "v", "", "The version of the Radius runtime to install, for example: 0.3.0")
}
