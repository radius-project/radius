/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package helm

import (
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	containerdErrors "github.com/containerd/containerd/remotes/errors"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	helmDriverSecret = "secret"
	installTimeout   = time.Duration(600) * time.Second
	uninstallTimeout = time.Duration(300) * time.Second
	retryTimeout     = time.Duration(10) * time.Second
	retries          = 5
)

// HelmConfig initializes a helm configuration object and sets the backend storage driver to use kubernetes secrets,
// returning the configuration object and an error if one occurs.
func HelmConfig(builder *strings.Builder, flags *genericclioptions.ConfigFlags) (*helm.Configuration, error) {
	hc := helm.Configuration{}
	// helmDriver is "secret" to make the backend storage driver
	// use kubernetes secrets.
	err := hc.Init(flags, *flags.Namespace, helmDriverSecret, func(format string, v ...any) {
		builder.WriteString(fmt.Sprintf(format, v...))
		builder.WriteRune('\n')
	})
	return &hc, err
}

func createTempDir() (string, error) {
	dir, err := os.MkdirTemp("", radiusReleaseName)
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %s", err)
	}
	return dir, nil
}

func locateChartFile(dirPath string) (string, error) {
	files, err := os.ReadDir(dirPath)
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

func helmChartFromContainerRegistry(version string, config *helm.Configuration, repoUrl string, releaseName string) (*chart.Chart, error) {
	pull := helm.NewPull()
	pull.Settings = &cli.EnvSettings{}
	pullopt := helm.WithConfig(config)
	pullopt(pull)

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

	var chartRef string

	if !registry.IsOCI(repoUrl) {
		// For non-OCI registries (like contour), we need to set the repo URL
		// to the registry URL. The chartRef is the release name.
		// ex.
		// pull.RepoURL = https://charts.bitnami.com/bitnami
		// pull.Run("contour")
		pull.RepoURL = repoUrl
		chartRef = releaseName
	} else {
		// For OCI registries (like radius), we will use the
		// repo URL + the releaseName as the chartRef.
		// pull.Run("oci://ghcr.io/radius-project/helm-chart/radius")
		chartRef = fmt.Sprintf("%s/%s", repoUrl, releaseName)

		// Since we are using an OCI registry, we need to set the registry client
		registryClient, err := registry.NewClient()
		if err != nil {
			return nil, err
		}

		pull.SetRegistryClient(registryClient)
	}

	_, err = pull.Run(chartRef)
	if err != nil {
		// Error handling for a specific case where credentials are stale.
		// https://github.com/helm/helm/issues/12584
		var errUnexpectedStatus containerdErrors.ErrUnexpectedStatus
		if errors.As(err, &errUnexpectedStatus) {
			unwrappedErr := UnwrapAll(err)
			unexpectedStatusErr, ok := unwrappedErr.(containerdErrors.ErrUnexpectedStatus)
			if ok {
				if unexpectedStatusErr.StatusCode == http.StatusForbidden {
					return nil, fmt.Errorf("recieved 403 unauthorized when downloading helm chart from the registry. you may want to perform a `docker logout` and re-try the command")
				}
			}
		}

		return nil, fmt.Errorf("error downloading helm chart from the registry for version: %s, release name: %s. Error: %w", version, releaseName, err)
	}

	chartPath, err := locateChartFile(dir)
	if err != nil {
		return nil, err
	}
	return loader.Load(chartPath)
}

func runInstall(installClient *helm.Install, helmChart *chart.Chart) error {
	var err error
	for i := 0; i < retries; i++ {
		_, err = installClient.Run(helmChart, helmChart.Values)
		if err == nil {
			return nil
		}
		time.Sleep(retryTimeout)
	}
	return err
}

func runUpgrade(upgradeClient *helm.Upgrade, releaseName string, helmChart *chart.Chart) error {
	var err error
	for i := 0; i < retries; i++ {
		_, err = upgradeClient.Run(releaseName, helmChart, helmChart.Values)
		if err == nil {
			return nil
		}
		time.Sleep(retryTimeout)
	}
	return err
}

func UnwrapAll(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}
