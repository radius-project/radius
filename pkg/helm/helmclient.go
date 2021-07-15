package helm

import (
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/Azure/radius/pkg/rad/logger"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	storageerrors "k8s.io/helm/pkg/storage/errors"
)

const (
	radiusReleaseName     = "radius"
	radiusHelmRepo        = "https://radius.azurecr.io/helm/v1/repo"
	RadiusSystemNamespace = "radius-system"
	helmDriverSecret      = "secret"
)

func ApplyRadiusHelmChart(version string) error {
	// For capturing output from helm.
	output := make(chan string)
	done := make(chan bool)
	helmConf, err := helmConfig(RadiusSystemNamespace, output, done)
	if err != nil {
		// Note: this is assuming that no logs from helm will be written
		helmOutput := getOutputFromChannel(output)
		return fmt.Errorf("failed to get helm config, err: %w, helm output: %s", err, helmOutput)
	}

	radiusChart, err := radiusChart(version, helmConf)
	if err != nil {
		done <- true
		helmOutput := getOutputFromChannel(output)
		return fmt.Errorf("failed to get radius chart, err: %w, helm output: %s", err, helmOutput)
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

	if err == storageerrors.ErrReleaseNotFound(radiusReleaseName) {
		logger.LogInfo("Installing new Radius Kubernetes environment to namespace: %s", RadiusSystemNamespace)

		err = runHelmInstall(helmConf, radiusChart)
		if err != nil {
			done <- true
			helmOutput := getOutputFromChannel(output)
			return fmt.Errorf("failed to run helm install, err: %w, helm output: %s", err, helmOutput)
		}
	} else if err == nil {
		logger.LogInfo("Found existing Radius Kubernetes environment, upgrading")

		if err != nil {
			done <- true
			helmOutput := getOutputFromChannel(output)
			return fmt.Errorf("failed to run helm upgrade, err: %w, helm output: %s", err, helmOutput)
		}
	}

	close(output)
	return err
}

func runRadiusHelmUpgrade(helmConf *helm.Configuration, radiusChart *chart.Chart) error {
	upgradeClient := helm.NewUpgrade(helmConf)
	upgradeClient.Namespace = RadiusSystemNamespace

	_, err := upgradeClient.Run(radiusReleaseName, radiusChart, radiusChart.Values)
	return err
}

func runHelmInstall(helmConf *helm.Configuration, radiusChart *chart.Chart) error {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = radiusReleaseName
	installClient.Namespace = RadiusSystemNamespace
	_, err := installClient.Run(radiusChart, radiusChart.Values)
	return err
}

func helmConfig(namespace string, output chan string, done chan bool) (*helm.Configuration, error) {
	hc := helm.Configuration{}
	flags := &genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}

	// Create a temp channel to be able to handle signal for completion
	temp := make(chan string)

	go func() {
		for {
			select {
			case msg := <-temp:
				output <- msg
			case <-done:
				// Allowing the string to be terminated
				// Don't need to close temp as channels don't need to be closed
				// and closing outside of the sender is a bad idea.
				close(output)
				return
			}
		}
	}()

	// helmDriver is "secret" to make the backend storage driver
	// use kubernetes secrets.
	err := hc.Init(flags, namespace, helmDriverSecret, func(format string, v ...interface{}) {
		temp <- fmt.Sprintf(format, v...)
	})
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

func getOutputFromChannel(output chan string) string {
	var sb strings.Builder

	for {
		select {
		case msg, done := <-output:
			if done {
				return sb.String()
			}
			sb.WriteString(msg)
		}
	}
}
