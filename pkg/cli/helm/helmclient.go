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

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
)

const (
	// DefaultInstallTimeout is the timeout applied to `rad install kubernetes` and
	// `rad upgrade kubernetes` when the caller does not specify one. Installing Radius on a
	// cold cluster has to pull every control-plane image before any Deployment can become
	// ready, which on a slow or throttled network can exceed a short timeout and surface as a
	// spurious "context deadline exceeded" failure. A healthy install completes in about a
	// minute, so ten minutes leaves ample headroom for a slow network while still failing in
	// reasonable time; use the --timeout flag when even that is not enough.
	// See https://github.com/radius-project/radius/issues/10236.
	DefaultInstallTimeout = time.Duration(10) * time.Minute

	uninstallTimeout = time.Duration(5) * time.Minute
	rollbackTimeout  = time.Duration(5) * time.Minute
)

//go:generate go tool mockgen -typed -source=helmclient.go -destination=./mock_helmclient.go -package=helm -imports helm=helm.sh/helm/v4/pkg/action,chart=helm.sh/helm/v4/pkg/chart/v2,release=helm.sh/helm/v4/pkg/release,releasev1=helm.sh/helm/v4/pkg/release/v1

// HelmClient is an interface for interacting with Helm charts.
type HelmClient interface {
	// RunHelmInstall installs the Helm chart using the supplied user-values map as the
	// override set. The map should contain only user-supplied overrides (not the chart
	// defaults); Helm merges them on top of the chart's defaults at render time.
	// A non-positive timeout falls back to DefaultInstallTimeout.
	RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, timeout time.Duration) (*releasev1.Release, error)

	// RunHelmUpgrade upgrades the Helm chart. When reuseValues is true,
	// upgrade uses ResetThenReuseValues semantics: it starts from the new chart's
	// defaults, overlays the user-supplied values stored on the previous release, and
	// then overlays vals. When reuseValues is false, the previous release's stored
	// user values are discarded and only vals (merged over the new chart's defaults)
	// are used.
	//
	// See https://helm.sh/docs/helm/helm_upgrade/#options for details on
	// --reset-then-reuse-values behavior.
	//
	// A non-positive timeout falls back to DefaultInstallTimeout.
	RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, reuseValues bool, timeout time.Duration) (*releasev1.Release, error)

	// RunHelmUninstall uninstalls the Helm chart.
	RunHelmUninstall(helmConf *helm.Configuration, releaseName, namespace string, wait bool) (*release.UninstallReleaseResponse, error)

	// RunHelmList lists the Helm releases.
	RunHelmList(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error)

	// RunHelmGet retrieves the Helm release information.
	RunHelmGet(helmConf *helm.Configuration, releaseName string) (*releasev1.Release, error)

	// RunHelmHistory retrieves the history of a Helm release.
	RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error)

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
// The vals map should contain only user-supplied overrides; Helm merges them on top of
// the chart's defaults during rendering.
// A non-positive timeout falls back to DefaultInstallTimeout.
func (client *HelmClientImpl) RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, timeout time.Duration) (*releasev1.Release, error) {
	installClient := newInstallClient(helmConf, releaseName, namespace, wait, timeout)

	if vals == nil {
		vals = map[string]any{}
	}

	rel, err := installClient.Run(helmChart, vals)
	if err != nil {
		return nil, err
	}

	return asRelease(rel)
}

// newInstallClient builds the Helm install action used by RunHelmInstall. It is separated from
// RunHelmInstall so the wait strategy and timeout configuration can be asserted without a cluster.
func newInstallClient(helmConf *helm.Configuration, releaseName, namespace string, wait bool, timeout time.Duration) *helm.Install {
	installClient := helm.NewInstall(helmConf)
	installClient.ReleaseName = releaseName
	installClient.Namespace = namespace
	installClient.CreateNamespace = true
	installClient.Timeout = effectiveTimeout(timeout)
	installClient.WaitStrategy = waitStrategy(wait)

	return installClient
}

// RunHelmUpgrade upgrades an existing Helm release with a new chart version or configuration.
// It optionally waits for the deployment to be ready.
// A non-positive timeout falls back to DefaultInstallTimeout.
// See https://helm.sh/docs/helm/helm_upgrade/#options for details on --reset-then-reuse-values behavior.
func (client *HelmClientImpl) RunHelmUpgrade(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, reuseValues bool, timeout time.Duration) (*releasev1.Release, error) {
	upgradeClient := newUpgradeClient(helmConf, namespace, wait, reuseValues, timeout)

	if vals == nil {
		vals = map[string]any{}
	}

	rel, err := upgradeClient.Run(releaseName, helmChart, vals)
	if err != nil {
		return nil, err
	}

	return asRelease(rel)
}

// newUpgradeClient builds the Helm upgrade action used by RunHelmUpgrade. It is separated from
// RunHelmUpgrade so the wait strategy and timeout configuration can be asserted without a cluster.
func newUpgradeClient(helmConf *helm.Configuration, namespace string, wait bool, reuseValues bool, timeout time.Duration) *helm.Upgrade {
	upgradeClient := helm.NewUpgrade(helmConf)
	upgradeClient.Namespace = namespace
	upgradeClient.WaitStrategy = waitStrategy(wait)
	upgradeClient.Timeout = effectiveTimeout(timeout)
	// ResetThenReuseValues is the desired default for Radius upgrades: pick up new chart defaults but preserve
	// any user overrides previously stored on the release. When the caller opts out, use ResetValues semantics.
	upgradeClient.ResetThenReuseValues = reuseValues
	upgradeClient.ResetValues = !reuseValues

	return upgradeClient
}

// RunHelmUninstall removes a Helm release and its associated resources from the cluster.
// It optionally waits for all resources to be deleted before returning.
func (client *HelmClientImpl) RunHelmUninstall(helmConf *helm.Configuration, releaseName, namespace string, wait bool) (*release.UninstallReleaseResponse, error) {
	uninstallClient := helm.NewUninstall(helmConf)
	uninstallClient.Timeout = uninstallTimeout
	uninstallClient.WaitStrategy = waitStrategy(wait)

	return uninstallClient.Run(releaseName)
}

// RunHelmList lists Helm releases that match the provided filter.
// It searches for deployed releases across all namespaces.
func (client *HelmClientImpl) RunHelmList(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error) {
	listClient := helm.NewList(helmConf)
	listClient.Filter = releaseName
	listClient.Deployed = true
	listClient.AllNamespaces = true

	releases, err := listClient.Run()
	if err != nil {
		return nil, err
	}

	return asReleases(releases)
}

// RunHelmGet retrieves detailed information about a specific Helm release.
// It returns the latest revision of the release.
func (client *HelmClientImpl) RunHelmGet(helmConf *helm.Configuration, releaseName string) (*releasev1.Release, error) {
	getClient := helm.NewGet(helmConf)
	getClient.Version = 0

	rel, err := getClient.Run(releaseName)
	if err != nil {
		return nil, err
	}

	return asRelease(rel)
}

// RunHelmHistory retrieves the revision history of a Helm release.
// It returns all revisions of the release, including superseded and failed deployments.
func (client *HelmClientImpl) RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error) {
	historyClient := helm.NewHistory(helmConf)
	historyClient.Max = 0 // Get all revisions

	releases, err := historyClient.Run(releaseName)
	if err != nil {
		return nil, err
	}

	return asReleases(releases)
}

// RunHelmPull downloads a Helm chart from a repository to the local filesystem.
// It returns the path to the downloaded chart archive.
func (client *HelmClientImpl) RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error) {
	pullClient := helm.NewPull(
		pullopts...,
	)

	return pullClient.Run(chartRef)
}

// RunHelmRollback rolls back a Helm release to a previous revision.
// It optionally waits for the rollback to complete and all resources to be ready.
func (client *HelmClientImpl) RunHelmRollback(helmConf *helm.Configuration, releaseName string, revision int, wait bool) error {
	rollbackClient := helm.NewRollback(helmConf)
	rollbackClient.Timeout = rollbackTimeout
	rollbackClient.WaitStrategy = waitStrategy(wait)
	rollbackClient.Version = revision

	return rollbackClient.Run(releaseName)
}

// LoadChart loads a Helm chart from the specified file path.
// The path can be a directory containing chart files or a packaged chart archive.
func (client *HelmClientImpl) LoadChart(chartPath string) (*chart.Chart, error) {
	return loader.Load(chartPath)
}

// waitStrategy translates the legacy boolean wait flag into the Helm v4 kube.WaitStrategy.
// A true value waits for all resources to become ready (the v3 "--wait" behavior), while a
// false value only waits for hooks (the v3 default when "--wait" was omitted).
//
// We deliberately use kube.LegacyStrategy rather than the Helm v4 default
// kube.StatusWatcherStrategy. The kstatus-backed watcher can miss the readiness transition of
// a resource and report it as InProgress until the timeout expires even though the cluster
// reports it as ready, which surfaces as a spurious failure such as:
//
//	resource Deployment/radius-system/ucp not ready. status: InProgress, message: Available: 0/1
//
// See https://github.com/helm/helm/issues/31526 and
// https://github.com/radius-project/radius/issues/12975. The legacy poller is sufficient for
// the Radius chart because every kind whose readiness gates a usable control plane
// (Deployment, StatefulSet, Service, Pod and the hook Jobs) is understood by Helm's legacy
// ReadyChecker; kinds it does not recognize are treated as immediately ready, which matches
// the behavior Radius relied on before the Helm v4 migration. Revisit once the upstream
// watcher bug is fixed.
func waitStrategy(wait bool) kube.WaitStrategy {
	if wait {
		return kube.LegacyStrategy
	}

	return kube.HookOnlyStrategy
}

// effectiveTimeout returns the caller-supplied timeout, falling back to DefaultInstallTimeout
// when the caller did not specify one (the zero value) or supplied a non-positive duration.
// Helm treats a non-positive timeout as "expire immediately", so guarding here keeps a
// zero-valued ChartOptions from failing the operation instantly.
func effectiveTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultInstallTimeout
	}

	return timeout
}

// asRelease converts a release.Releaser returned by the Helm v4 action API into the concrete
// *releasev1.Release type used throughout this package.
func asRelease(r release.Releaser) (*releasev1.Release, error) {
	if r == nil {
		return nil, nil
	}

	rel, ok := r.(*releasev1.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type %T returned by helm", r)
	}

	return rel, nil
}

// asReleases converts a slice of release.Releaser into concrete *releasev1.Release values.
func asReleases(rs []release.Releaser) ([]*releasev1.Release, error) {
	releases := make([]*releasev1.Release, 0, len(rs))
	for _, r := range rs {
		rel, err := asRelease(r)
		if err != nil {
			return nil, err
		}

		releases = append(releases, rel)
	}

	return releases, nil
}
