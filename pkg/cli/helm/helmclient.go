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
	"fmt"
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
	fmt.Printf("RunHelmList: Looking for release with name=%s\n", releaseName)

	listClient := helm.NewList(helmConf)
	listClient.Filter = releaseName
	// Don't filter by deployed status - we want to find the release in any state
	// listClient.Deployed = true

	fmt.Printf("RunHelmList: Filter=%s, Deployed=%v\n", listClient.Filter, listClient.Deployed)

	// Debug: Try to get a specific release by name
	getClient := helm.NewGet(helmConf)
	release, err := getClient.Run(releaseName)
	if err != nil {
		fmt.Printf("RunHelmList: Failed to get release %s directly: %v\n", releaseName, err)

		// During pre-upgrade hooks, the release might be locked. Try getting history instead
		fmt.Printf("RunHelmList: Attempting to get release history for %s\n", releaseName)
		historyClient := helm.NewHistory(helmConf)
		historyClient.Max = 1 // Just get the latest
		history, histErr := historyClient.Run(releaseName)
		if histErr != nil {
			fmt.Printf("RunHelmList: Failed to get history for %s: %v\n", releaseName, histErr)
		} else if len(history) > 0 {
			fmt.Printf("RunHelmList: Found %d history entries for %s\n", len(history), releaseName)
			latestHistory := history[len(history)-1]
			fmt.Printf("RunHelmList: Latest history: Name=%s, Status=%s, Version=%d\n", latestHistory.Name, latestHistory.Info.Status, latestHistory.Version)
		}
	} else if release != nil {
		fmt.Printf("RunHelmList: Successfully got release %s directly: Status=%s, Version=%d\n", release.Name, release.Info.Status, release.Version)
	}

	// Let's also list ALL releases to see what's there
	allListClient := helm.NewList(helmConf)
	// Don't filter by deployed status
	// allListClient.Deployed = true
	allListClient.AllNamespaces = false // Make sure we're not searching all namespaces
	allReleases, allErr := allListClient.Run()
	fmt.Printf("RunHelmList: Listing ALL releases (Deployed=%v, AllNamespaces=%v)\n", allListClient.Deployed, allListClient.AllNamespaces)
	if allErr != nil {
		fmt.Printf("RunHelmList: Error listing all releases: %v\n", allErr)
	} else {
		fmt.Printf("RunHelmList: Found %d total releases\n", len(allReleases))
		for i, rel := range allReleases {
			if rel != nil {
				fmt.Printf("  [%d]: Name=%s, Namespace=%s, Status=%s, Version=%d\n", i, rel.Name, rel.Namespace, rel.Info.Status, rel.Version)
			}
		}
	}

	// Debug: Additional information about the Helm configuration
	fmt.Printf("RunHelmList: Additional debug - checking if we can access releases at all\n")
	if helmConf.Releases == nil {
		fmt.Printf("RunHelmList: WARNING - helmConf.Releases is nil!\n")
	} else {
		fmt.Printf("RunHelmList: helmConf.Releases is initialized\n")
	}

	// Also try without the Deployed filter
	allListClient2 := helm.NewList(helmConf)
	allListClient2.AllNamespaces = false
	allReleases2, _ := allListClient2.Run()
	fmt.Printf("RunHelmList: Without Deployed filter, found %d releases\n", len(allReleases2))
	for i, rel := range allReleases2 {
		if rel != nil {
			fmt.Printf("  [%d]: Name=%s, Status=%s\n", i, rel.Name, rel.Info.Status)
		}
	}

	releases, err := listClient.Run()
	if err != nil {
		fmt.Printf("RunHelmList: error from listClient.Run(): %v\n", err)
	} else {
		fmt.Printf("RunHelmList: Filtered results returned %d releases\n", len(releases))
		for i, rel := range releases {
			if rel != nil {
				fmt.Printf("RunHelmList: Release[%d]: Name=%s, Namespace=%s, Status=%s\n", i, rel.Name, rel.Namespace, rel.Info.Status)
			}
		}
	}

	return releases, err
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
