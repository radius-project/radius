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
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	containerderrors "github.com/containerd/containerd/remotes/errors"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/registry"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	helmDriverSecret = "secret"
)

// ChartOptions describes the options for a Helm chart.
type ChartOptions struct {
	// Target namespace for deployment
	Namespace string

	// ReleaseName specifies the release name for the helm chart.
	ReleaseName string

	// ChartRepo specifies the helm chart repository.
	ChartRepo string

	// Reinstall specifies whether to reinstall the chart (helm upgrade).
	Reinstall bool

	// ChartPath specifies an override for the chart location.
	ChartPath string

	// ChartVersion specifies the chart version.
	ChartVersion string

	// SetArgs specifies as set of additional "values" to pass to helm. These are specified using the command-line syntax accepted
	// by helm, in the order they appear on the command line (last one wins).
	SetArgs []string

	// SetFileArgs specifies as set of additional "values" from file to pass it to helm.
	SetFileArgs []string
}

// HelmAction is an interface for performing actions on Helm charts.
type HelmAction interface {
	// HelmChartFromContainerRegistry downloads a helm chart (using helm pull) from a container registry and returns the chart object.
	HelmChartFromContainerRegistry(version string, config *helm.Configuration, repoUrl string, releaseName string) (*chart.Chart, error)

	// ApplyHelmChart checks if a Helm chart is already installed, and if not, installs it or upgrades it if the "Reinstall" option is set.
	ApplyHelmChart(kubeContext string, helmChart *chart.Chart, helmConf *helm.Configuration, options ChartOptions) error

	// QueryRelease checks to see if a release is deployed to a namespace for a given kubecontext.
	// Returns a bool indicating if the release is deployed, the version of the release, and an error if one occurs.
	QueryRelease(kubeContext, namespace, releaseName string) (bool, string, error)

	// LoadChart loads a helm chart from the specified path and returns the chart object.
	LoadChart(chartPath string) (*chart.Chart, error)
}

type HelmActionImpl struct {
	HelmClient HelmClient
}

var _ HelmAction = &HelmActionImpl{}

func NewHelmAction(helmClient HelmClient) *HelmActionImpl {
	return &HelmActionImpl{HelmClient: helmClient}
}

func (helmAction *HelmActionImpl) HelmChartFromContainerRegistry(version string, config *helm.Configuration, repoUrl string, releaseName string) (*chart.Chart, error) {
	pullopts := []helm.PullOpt{
		helm.WithConfig(config),
		func(p *helm.Pull) {
			p.Settings = &cli.EnvSettings{}
		},
	}

	// If version isn't set, it will use the latest version.
	if version != "" {
		pullopts = append(pullopts, func(p *helm.Pull) {
			p.Version = version
		})
	} else {
		// Support prerelease builds when the version is unspecified. We always specify
		// the version for a release build.
		pullopts = append(pullopts, func(p *helm.Pull) {
			p.Devel = true
		})
	}

	dir, err := createTempDir()
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	pullopts = append(pullopts, func(p *helm.Pull) {
		p.DestDir = dir
	})

	var chartRef string

	if !registry.IsOCI(repoUrl) {
		// For non-OCI registries (like contour), we need to set the repo URL
		// to the registry URL. The chartRef is the release name.
		// ex.
		// pull.RepoURL = https://charts.bitnami.com/bitnami
		// pull.Run("contour")
		pullopts = append(pullopts, func(p *helm.Pull) {
			p.RepoURL = repoUrl
		})
		chartRef = releaseName
	} else {
		// For OCI registries (like radius), we will use the
		// repo URL + the releaseName as the chartRef.
		// ex.
		// pull.Run("oci://ghcr.io/radius-project/helm-chart/radius")
		chartRef = fmt.Sprintf("%s/%s", repoUrl, releaseName)

		// Since we are using an OCI registry, we need to set the registry client
		registryClient, err := registry.NewClient()
		if err != nil {
			return nil, err
		}

		pullopts = append(pullopts, func(p *helm.Pull) {
			p.SetRegistryClient(registryClient)
		})
	}

	_, err = helmAction.HelmClient.RunHelmPull(pullopts, chartRef)
	if err != nil {
		// Error handling for a specific case where credentials are stale.
		// This happens for ghcr in particular because ghcr does not use
		// subdomains - the scope of a login is all of ghcr.io.
		// https://github.com/helm/helm/issues/12584
		if isHelmGHCR403Error(err) {
			return nil, clierrors.Message("received 403 unauthorized when downloading helm chart from the registry. you may want to perform a `docker logout ghcr.io` and re-try the command")
		}

		return nil, clierrors.MessageWithCause(err, "error downloading helm chart from the registry for version: %s, release name: %s", version, releaseName)
	}

	chartPath, err := locateChartFile(dir)
	if err != nil {
		return nil, err
	}

	return helmAction.LoadChart(chartPath)
}

func (helmAction *HelmActionImpl) ApplyHelmChart(kubeContext string, helmChart *chart.Chart, helmConf *helm.Configuration, options ChartOptions) error {
	chartInstalled, _, err := helmAction.QueryRelease(kubeContext, options.ReleaseName, options.Namespace)
	if err != nil {
		return fmt.Errorf("failed to query Helm release, err: %w", err)
	}

	if !chartInstalled {
		_, err = helmAction.HelmClient.RunHelmInstall(helmConf, helmChart, options.ReleaseName, options.Namespace)
		if err != nil {
			return fmt.Errorf("failed to run Helm install, err: %w", err)
		}
	} else if options.Reinstall {
		_, err = helmAction.HelmClient.RunHelmUpgrade(helmConf, helmChart, options.ReleaseName, options.Namespace)
		if err != nil {
			return fmt.Errorf("failed to run Helm upgrade, err: %w", err)
		}
	}

	return nil
}

func (helmAction *HelmActionImpl) QueryRelease(kubeContext, releaseName, namespace string) (bool, string, error) {
	flags := genericclioptions.ConfigFlags{
		Namespace: &namespace,
		Context:   &kubeContext,
	}

	helmConf, err := initHelmConfig(&flags)
	if err != nil {
		return false, "", fmt.Errorf("failed to get helm config, err: %w", err)
	}

	releases, err := helmAction.HelmClient.RunHelmList(helmConf, releaseName, namespace)
	if err != nil {
		return false, "", fmt.Errorf("failed to run helm list, err: %w", err)
	}

	if len(releases) == 0 {
		return false, "", nil
	}

	if len(releases) > 1 {
		return false, "", fmt.Errorf("multiple deployed releases found with the same name: %s", releaseName)
	}

	// Get the latest deployed release (List returns sorted by revision number)
	latestRelease := releases[0]
	if latestRelease.Chart == nil || latestRelease.Chart.Metadata == nil {
		return false, "", fmt.Errorf("failed to get chart version for release: %s", releaseName)
	}

	return true, latestRelease.Chart.Metadata.Version, nil
}

func (helmAction *HelmActionImpl) LoadChart(chartPath string) (*chart.Chart, error) {
	return helmAction.HelmClient.LoadChart(chartPath)
}

// initHelmConfig initializes a helm configuration object and sets the backend storage driver to use kubernetes secrets,
// returning the configuration object and an error if one occurs.
func initHelmConfig(flags *genericclioptions.ConfigFlags) (*helm.Configuration, error) {
	builder := strings.Builder{}
	hc := helm.Configuration{}
	// helmDriver is "secret" to make the backend storage driver
	// use kubernetes secrets.
	err := hc.Init(flags, *flags.Namespace, helmDriverSecret, func(format string, v ...any) {
		builder.WriteString(fmt.Sprintf(format, v...))
		builder.WriteRune('\n')
	})
	return &hc, err
}

// createTempDir creates a temporary directory for the helm chart and returns the path to the directory.
func createTempDir() (string, error) {
	dir, err := os.MkdirTemp("", radiusReleaseName)
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %s", err)
	}
	return dir, nil
}

// locateChartFile locates the chart file in the specified directory and returns the path to the chart file.
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

// isHelmGHCR403Error is a helper function to determine if an error is a specific helm error
// (403 unauthorized when downloading a helm chart from ghcr.io) from a chain of errors.
func isHelmGHCR403Error(err error) bool {
	var errUnexpectedStatus containerderrors.ErrUnexpectedStatus
	if errors.As(err, &errUnexpectedStatus) {
		if errUnexpectedStatus.StatusCode == http.StatusForbidden && strings.Contains(errUnexpectedStatus.RequestURL, "ghcr.io") {
			return true
		}
	}

	return false
}
