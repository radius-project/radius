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
	"time"

	helm "helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
)

const (
	installTimeout   = time.Duration(5) * time.Minute
	uninstallTimeout = time.Duration(5) * time.Minute
	upgradeTimeout   = time.Duration(5) * time.Minute
	rollbackTimeout  = time.Duration(5) * time.Minute
)

//go:generate mockgen -typed -destination=./mock_helmclient.go -package=helm -self_package github.com/radius-project/radius/pkg/cli/helm github.com/radius-project/radius/pkg/cli/helm HelmClient

// HelmClient is an interface for interacting with Helm charts.
type HelmClient interface {
	// RunHelmInstall installs the Helm chart.
	RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error)

	// RunHelmUpgrade upgrades the Helm chart.
	RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error)

	// RunHelmUninstall uninstalls the Helm chart.
	RunHelmUninstall(helmConf *helm.Configuration, releaseName, namespace string, wait bool) (*release.UninstallReleaseResponse, error)

	// RunHelmList lists the Helm releases.
	RunHelmList(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error)

	// RunHelmGet retrieves the Helm release information.
	RunHelmGet(helmConf *helm.Configuration, releaseName string) (*release.Release, error)

	// RunHelmHistory retrieves the history of a Helm release.
	RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error)

	// RunHelmRollback rolls back a release to a previous revision.
	RunHelmRollback(helmConf *helm.Configuration, releaseName string, revision int, wait bool) error

	// RunHelmPull pulls the Helm chart.
	RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error)

	// LoadChart loads a Helm chart from the specified path.
	LoadChart(chartPath string) (*chart.Chart, error)
}

// HelmClientImpl is an implementation of the HelmClient interface.
// It uses the Helm go sdk to perform operations on Helm charts.
type HelmClientImpl struct{}

var _ HelmClient = &HelmClientImpl{}

// NewHelmClient creates a new instance of HelmClient that uses the Helm Go SDK
// to perform operations on Helm charts.
func NewHelmClient() HelmClient {
	return &HelmClientImpl{}
}

// RunHelmInstall installs a Helm chart as a new release in the specified namespace.
// It creates the namespace if it doesn't exist and optionally waits for the deployment to be ready.
func (client *HelmClientImpl) RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error) {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = releaseName
	installClient.Namespace = namespace
	installClient.CreateNamespace = true
	installClient.Timeout = installTimeout
	installClient.Wait = wait

	return installClient.Run(helmChart, helmChart.Values)
}

// RunHelmUpgrade upgrades an existing Helm release with a new chart version or configuration.
// It recreates pods to ensure the new configuration is applied and optionally waits for the deployment to be ready.
// The upgrade reuses existing release values and merges them with any new values provided in helmChart.Values.
func (client *HelmClientImpl) RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error) {
	upgradeClient := helm.NewUpgrade(helmConf)
	upgradeClient.Namespace = namespace
	upgradeClient.Wait = wait
	upgradeClient.Timeout = upgradeTimeout
	upgradeClient.Recreate = true
	upgradeClient.ReuseValues = true

	return upgradeClient.Run(releaseName, helmChart, helmChart.Values)
}

// RunHelmUninstall removes a Helm release and its associated resources from the cluster.
// It optionally waits for all resources to be deleted before returning.
func (client *HelmClientImpl) RunHelmUninstall(helmConf *helm.Configuration, releaseName, namespace string, wait bool) (*release.UninstallReleaseResponse, error) {
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = wait

	return uninstallClient.Run(releaseName)
}

// RunHelmList lists Helm releases that match the provided filter.
// It searches for deployed releases across all namespaces.
func (client *HelmClientImpl) RunHelmList(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error) {
	listClient := helm.NewList(helmConf)
	listClient.Filter = releaseName
	listClient.Deployed = true
	listClient.AllNamespaces = true

	return listClient.Run()
}

// RunHelmGet retrieves detailed information about a specific Helm release.
// It returns the latest revision of the release.
func (client *HelmClientImpl) RunHelmGet(helmConf *helm.Configuration, releaseName string) (*release.Release, error) {
	getClient := helm.NewGet(helmConf)
	getClient.Version = 0

	return getClient.Run(releaseName)
}

// RunHelmHistory retrieves the revision history of a Helm release.
// It returns all revisions of the release, including superseded and failed deployments.
func (client *HelmClientImpl) RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error) {
	historyClient := helm.NewHistory(helmConf)
	historyClient.Max = 0 // Get all revisions

	return historyClient.Run(releaseName)
}

// RunHelmPull downloads a Helm chart from a repository to the local filesystem.
// It returns the path to the downloaded chart archive.
func (client *HelmClientImpl) RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error) {
	pullClient := helm.NewPullWithOpts(
		pullopts...,
	)

	return pullClient.Run(chartRef)
}

// RunHelmRollback rolls back a Helm release to a previous revision.
// It optionally waits for the rollback to complete and all resources to be ready.
func (client *HelmClientImpl) RunHelmRollback(helmConf *helm.Configuration, releaseName string, revision int, wait bool) error {
	rollbackClient := helm.NewRollback(helmConf)
	rollbackClient.Timeout = rollbackTimeout
	rollbackClient.Wait = wait
	rollbackClient.Version = revision

	return rollbackClient.Run(releaseName)
}

// LoadChart loads a Helm chart from the specified file path.
// The path can be a directory containing chart files or a packaged chart archive.
func (client *HelmClientImpl) LoadChart(chartPath string) (*chart.Chart, error) {
	return loader.Load(chartPath)
}
