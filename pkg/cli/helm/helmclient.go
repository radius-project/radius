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

	// RunHelmHistory lists the release revisions for a given release.
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

func NewHelmClient() HelmClient {
	return &HelmClientImpl{}
}

func (client *HelmClientImpl) RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error) {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = releaseName
	installClient.Namespace = namespace
	installClient.CreateNamespace = true
	installClient.Timeout = installTimeout
	installClient.Wait = wait

	return installClient.Run(helmChart, helmChart.Values)
}

func (client *HelmClientImpl) RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, releaseName, namespace string, wait bool) (*release.Release, error) {
	upgradeClient := helm.NewUpgrade(helmConf)
	upgradeClient.Namespace = namespace
	upgradeClient.Wait = wait
	upgradeClient.Timeout = upgradeTimeout
	upgradeClient.Recreate = true

	return upgradeClient.Run(releaseName, helmChart, helmChart.Values)
}

func (client *HelmClientImpl) RunHelmUninstall(helmConf *helm.Configuration, releaseName, namespace string, wait bool) (*release.UninstallReleaseResponse, error) {
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.Wait = wait

	return uninstallClient.Run(releaseName)
}

func (client *HelmClientImpl) RunHelmList(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error) {
	listClient := helm.NewList(helmConf)
	listClient.Filter = releaseName

	return listClient.Run()
}

func (client *HelmClientImpl) RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error) {
	pullClient := helm.NewPullWithOpts(
		pullopts...,
	)

	return pullClient.Run(chartRef)
}

func (client *HelmClientImpl) RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*release.Release, error) {
	historyClient := helm.NewHistory(helmConf)
	historyClient.Max = 256 // Helm default

	return historyClient.Run(releaseName)
}

func (client *HelmClientImpl) RunHelmRollback(helmConf *helm.Configuration, releaseName string, revision int, wait bool) error {
	rollbackClient := helm.NewRollback(helmConf)
	rollbackClient.Timeout = rollbackTimeout
	rollbackClient.Wait = wait
	rollbackClient.Version = revision

	return rollbackClient.Run(releaseName)
}

func (client *HelmClientImpl) LoadChart(chartPath string) (*chart.Chart, error) {
	return loader.Load(chartPath)
}
