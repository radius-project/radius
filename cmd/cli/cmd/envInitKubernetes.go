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
	"os"
	"path/filepath"

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
		name, err := cmd.Flags().GetString("environment")
		if err != nil {
			return err
		}

		interactive, err := cmd.Flags().GetBool("interactive")
		if err != nil {
			return err
		}

		namespace := "default"
		if interactive {
			namespace, err = choseNamespace(cmd.Context())
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
			Namespace: "radius-system",
			Version:   version.Version(),
		})
		if err != nil {
			return err
		}
		logger.CompleteStep(step)

		v := viper.GetViper()
		env, err := rad.ReadEnvironmentSection(v)
		if err != nil {
			return err
		}

		if name == "" {
			name = k8sconfig.CurrentContext
		}

		env.Items[name] = map[string]interface{}{
			"kind":      environments.KindKubernetes,
			"context":   k8sconfig.CurrentContext,
			"namespace": namespace,
		}

		logger.LogInfo("using environment %v", name)
		env.Default = name
		rad.UpdateEnvironmentSection(v, env)

		err = rad.SaveConfig()
		if err != nil {
			return err
		}

		return nil
	},
}

func init() {
	envInitCmd.AddCommand(envInitKubernetesCmd)
	envInitKubernetesCmd.Flags().BoolP("interactive", "i", false, "Specify interactive to choose namespace interactively")
	envInitKubernetesCmd.Flags().StringP("namespace", "n", "radius-system", "The Kubernetes namespace to install Radius in")
}

func choseNamespace(ctx context.Context) (string, error) {
	name, err := prompt.Text("Enter a Resource Group name (empty to default to 'radius-system' namespace):", prompt.EmptyValidator)
	if name == "" {
		name = "default"
	}
	return name, err
}

type KubernetesInitConfig struct {
	Namespace string
	Version   string
}

func createNamespace(ctx context.Context, namespace string) error {
	// GetClient for kubernetes
	client, err := utils.GetKubernetesClient()
	if err != nil {
		return fmt.Errorf("can't connect to a Kubernetes cluster: %v", err)
	}

	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	// Ignore failures if namespace already exists
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	return nil
}

func helmConfig(namespace string) (*helm.Configuration, error) {
	ac := helm.Configuration{}
	flags := &genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}
	err := ac.Init(flags, namespace, "secret", ignoreLog)
	return &ac, err
}

func ignoreLog(format string, v ...interface{}) {
}

func createTempDir() (string, error) {
	dir, err := os.MkdirTemp("", "radius")
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %s", err)
	}
	return dir, nil
}

func radiusChart(version string, config *helm.Configuration) (*chart.Chart, error) {
	dir, err := createTempDir()
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(dir)

	// Iterate through everything in the chart and write it to a
	// temp folder, then load the helm chart.
	err = fs.WalkDir(chartFolder, "Chart", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			// Make sure directory exists in temp directory.
			err := os.MkdirAll(filepath.Join(dir, path), os.ModePerm)
			return err
		}

		file, err := chartFolder.Open(path)
		if err != nil {
			return err
		}
		stat, err := file.Stat()
		if err != nil {
			return err
		}

		totalLen := int(stat.Size())
		buffer := make([]byte, totalLen)

		for runningLength := 0; runningLength != totalLen; {
			temp, err := file.Read(buffer)
			if err != nil {
				return err
			}
			runningLength += temp
		}

		fullPath := filepath.Join(dir, path)

		err = os.WriteFile(fullPath, buffer, os.ModePerm)

		return err
	})

	if err != nil {
		return nil, err
	}

	loader, err := loader.LoadDir(filepath.Join(dir, "Chart"))
	if err != nil {
		return nil, err
	}
	return loader, nil
}

// RunCLICommand runs a kubectl CLI command with stdout and stderr forwarded to this process's output.
func install(ctx context.Context, config KubernetesInitConfig) error {
	// Create namespace if not present
	err := createNamespace(ctx, config.Namespace)
	if err != nil {
		return err
	}

	// Get helm chart (needs to either be in repo or outside)
	helmConf, err := helmConfig(config.Namespace)
	if err != nil {
		return err
	}

	radiusChart, err := radiusChart(config.Version, helmConf)
	if err != nil {
		return err
	}

	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = "radius"
	installClient.Namespace = config.Namespace

	if _, err = installClient.Run(radiusChart, radiusChart.Values); err != nil {
		return err
	}

	return err
}
