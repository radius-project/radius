// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package helm

import (
	_ "embed"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	helmDriverSecret = "secret"
)

func HelmConfig(namespace string, builder strings.Builder) (*helm.Configuration, error) {
	hc := helm.Configuration{}
	flags := &genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}

	// helmDriver is "secret" to make the backend storage driver
	// use kubernetes secrets.
	err := hc.Init(flags, namespace, helmDriverSecret, func(format string, v ...interface{}) {
		builder.WriteString(fmt.Sprintf(format, v...))
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

func helmChartFromRepo(version string, config *helm.Configuration, repoUrl string, releaseName string) (*chart.Chart, error) {
	pull := helm.NewPull()
	pull.RepoURL = repoUrl
	pull.Settings = &cli.EnvSettings{}

	// If version isn't set, it will use the latest version.
	if version != "" {
		pull.Version = version
	} else {
		// Support prerelease builds when the version is unspecified. We always specify
		// the version for a release build.
		pull.Devel = true
	}

	dir, err := createTempDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	pull.DestDir = dir

	_, err = pull.Run(releaseName)
	if err != nil {
		return nil, err
	}

	chartPath, err := locateChartFile(dir)
	if err != nil {
		return nil, err
	}
	return loader.Load(chartPath)
}
