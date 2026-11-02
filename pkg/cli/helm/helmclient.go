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
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/downloader"
	"helm.sh/helm/v4/pkg/getter"
	"helm.sh/helm/v4/pkg/helmpath"
	"helm.sh/helm/v4/pkg/kube"
	"helm.sh/helm/v4/pkg/release"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	repo "helm.sh/helm/v4/pkg/repo/v1"
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

	// FieldManager is the server-side apply field manager for every change Radius makes through Helm.
	FieldManager = "rad"
)

// Helm 4 otherwise names the field manager after the running executable, so the same CLI saved as
// rad and as rad_linux_amd64 owns fields as two managers, and an upgrade that changes those fields
// fails with a conflict. See https://github.com/radius-project/radius/issues/13454.
func init() {
	kube.ManagedFieldsManager = FieldManager
}

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

	// RunHelmHistory retrieves the history of a Helm release in ascending revision order.
	RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error)

	// RunHelmRollback rolls back a release to a previous revision by re-rendering that revision's chart and
	// user values against the live cluster. Revision 0 means the revision before the latest one.
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
	upgradeClient := newUpgradeClient(helmConf, namespace, wait, reuseValues, timeout, usesServerSideApply(helmConf, releaseName))

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
func newUpgradeClient(helmConf *helm.Configuration, namespace string, wait bool, reuseValues bool, timeout time.Duration, serverSideApply bool) *helm.Upgrade {
	upgradeClient := helm.NewUpgrade(helmConf)
	upgradeClient.Namespace = namespace
	upgradeClient.WaitStrategy = waitStrategy(wait)
	upgradeClient.Timeout = effectiveTimeout(timeout)
	// ResetThenReuseValues is the desired default for Radius upgrades: pick up new chart defaults but preserve
	// any user overrides previously stored on the release. When the caller opts out, use ResetValues semantics.
	upgradeClient.ResetThenReuseValues = reuseValues
	upgradeClient.ResetValues = !reuseValues
	// Take over fields owned by other managers, such as an older CLI saved under a different file name, helm,
	// or kubectl, as client-side apply did with Helm 3. Helm rejects this option for client-side apply, which
	// releases first installed with Helm 3 still use and which already overwrites those fields.
	upgradeClient.ForceConflicts = serverSideApply

	return upgradeClient
}

// usesServerSideApply reports whether Helm will upgrade the release with server-side apply. Helm upgrades
// in "auto" mode, which keeps the apply method of the release's latest revision.
func usesServerSideApply(helmConf *helm.Configuration, releaseName string) bool {
	if helmConf.Releases == nil {
		return false
	}
	last, err := helmConf.Releases.Last(releaseName)
	if err != nil {
		return false
	}
	rel, err := asRelease(last)
	if err != nil || rel == nil {
		return false
	}
	return rel.ApplyMethod == string(releasev1.ApplyMethodServerSideApply)
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
// It returns all revisions in ascending revision order, including superseded and failed deployments.
func (client *HelmClientImpl) RunHelmHistory(helmConf *helm.Configuration, releaseName string) ([]*releasev1.Release, error) {
	historyClient := helm.NewHistory(helmConf)
	historyClient.Max = 0 // Get all revisions

	releases, err := historyClient.Run(releaseName)
	if err != nil {
		return nil, err
	}

	history, err := asReleases(releases)
	if err != nil {
		return nil, err
	}

	// Helm's history action preserves storage order, which may sort v10 before v9.
	releaseutil.SortByRevision(history)
	return history, nil
}

// RunHelmPull downloads a Helm chart from a repository to the local filesystem.
// It returns the output of the pull.
//
// When the pull options set a repository URL, RunHelmPull looks up the chart in the repository index and
// downloads the chart archive itself. Helm would download that index to the user's Helm cache, and fail when
// the cache isn't writable, so RunHelmPull downloads it to the pull's repository cache instead. Each request
// times out after chartPullRequestTimeout rather than Helm's default of two minutes.
func (client *HelmClientImpl) RunHelmPull(pullopts []helm.PullOpt, chartRef string) (string, error) {
	pullClient := helm.NewPull(
		pullopts...,
	)

	if pullClient.RepoURL == "" {
		return pullClient.Run(chartRef)
	}

	if pullClient.Verify || pullClient.VerifyLater || pullClient.Untar {
		return "", errors.New("verifying or unpacking a chart from a repository URL is not supported")
	}

	getters := getter.All(pullClient.Settings, getter.WithTimeout(chartPullRequestTimeout))
	chartURL, err := findChartInRepoURL(pullClient, chartRef, getters)
	if err != nil {
		return "", err
	}
	return downloadChart(pullClient, chartURL, getters)
}

// findChartInRepoURL returns the URL of the named chart in the pull's chart repository. It matches
// repo.FindChartInRepoURL, which Pull.Run uses for a repository URL, except that the repository index is
// downloaded to the pull's repository cache with the given getters.
func findChartInRepoURL(pull *helm.Pull, chartName string, getters getter.Providers) (string, error) {
	entry := &repo.Entry{
		Name:                  rand.Text(),
		URL:                   pull.RepoURL,
		Username:              pull.Username,
		Password:              pull.Password,
		PassCredentialsAll:    pull.PassCredentialsAll,
		CertFile:              pull.CertFile,
		KeyFile:               pull.KeyFile,
		CAFile:                pull.CaFile,
		InsecureSkipTLSVerify: pull.InsecureSkipTLSVerify,
	}
	chartRepo, err := repo.NewChartRepository(entry, getters)
	if err != nil {
		return "", err
	}
	chartRepo.CachePath = pull.Settings.RepositoryCache
	defer func() {
		os.RemoveAll(filepath.Join(chartRepo.CachePath, helmpath.CacheChartsFile(entry.Name)))
		os.RemoveAll(filepath.Join(chartRepo.CachePath, helmpath.CacheIndexFile(entry.Name)))
	}()

	indexPath, err := chartRepo.DownloadIndexFile()
	if err != nil {
		return "", fmt.Errorf("looks like %q is not a valid chart repository or cannot be reached: %w", pull.RepoURL, err)
	}
	index, err := repo.LoadIndexFile(indexPath)
	if err != nil {
		return "", err
	}

	chartDescription := fmt.Sprintf("chart %q", chartName)
	if pull.Version != "" {
		chartDescription = fmt.Sprintf("%s version %q", chartDescription, pull.Version)
	}
	chartVersion, err := index.Get(chartName, pull.Version)
	if err != nil {
		return "", repo.ChartNotFoundError{Chart: chartDescription, RepoURL: pull.RepoURL}
	}
	if len(chartVersion.URLs) == 0 {
		return "", fmt.Errorf("%s has no downloadable URLs", chartDescription)
	}

	chartURL, err := repo.ResolveReferenceURL(pull.RepoURL, chartVersion.URLs[0])
	if err != nil {
		return "", fmt.Errorf("failed to make chart URL absolute: %w", err)
	}
	return chartURL, nil
}

// downloadChart downloads the chart archive at chartURL to the pull's destination directory with the given
// getters. It matches what Pull.Run does for a chart URL when the chart isn't verified or unpacked.
func downloadChart(pull *helm.Pull, chartURL string, getters getter.Providers) (string, error) {
	var out strings.Builder
	chartDownloader := downloader.ChartDownloader{
		Out:     &out,
		Verify:  downloader.VerifyNever,
		Getters: getters,
		Options: []getter.Option{
			getter.WithBasicAuth(pull.Username, pull.Password),
			getter.WithPassCredentialsAll(pull.PassCredentialsAll),
			getter.WithTLSClientConfig(pull.CertFile, pull.KeyFile, pull.CaFile),
			getter.WithInsecureSkipVerifyTLS(pull.InsecureSkipTLSVerify),
			getter.WithPlainHTTP(pull.PlainHTTP),
		},
		RepositoryConfig: pull.Settings.RepositoryConfig,
		RepositoryCache:  pull.Settings.RepositoryCache,
		ContentCache:     pull.Settings.ContentCache,
	}
	_, _, err := chartDownloader.DownloadTo(chartURL, pull.Version, pull.DestDir)
	return out.String(), err
}

// RunHelmRollback rolls back a Helm release to a previous revision. Revision 0 means the revision before the
// latest one. It optionally waits for the rollback to complete and all resources to be ready.
//
// Instead of replaying the target revision's stored manifest like helm rollback, it upgrades the release to the
// target revision's chart and user values. Templates then render against the live cluster, so lookup reads the
// current encryption Secret and its resourceVersion, and the rollback takes over fields owned by other managers
// like an upgrade does. Helm records the new revision with the description "Rollback to <revision>".
func (client *HelmClientImpl) RunHelmRollback(helmConf *helm.Configuration, releaseName string, revision int, wait bool) error {
	if helmConf == nil || helmConf.Releases == nil {
		return fmt.Errorf("helm configuration is not initialized")
	}
	if revision < 0 {
		return fmt.Errorf("invalid revision %d: revision must be 0 or greater", revision)
	}
	if revision == 0 {
		last, err := helmConf.Releases.Last(releaseName)
		if err != nil {
			return err
		}
		current, err := asRelease(last)
		if err != nil {
			return err
		}
		if current.Version <= 1 {
			return fmt.Errorf("release %q has no previous revision to roll back to", releaseName)
		}
		revision = current.Version - 1
	}

	stored, err := helmConf.Releases.Get(releaseName, revision)
	if err != nil {
		return fmt.Errorf("failed to get revision %d of release %q: %w", revision, releaseName, err)
	}
	target, err := asRelease(stored)
	if err != nil {
		return err
	}
	if target.Chart == nil {
		return fmt.Errorf("revision %d of release %q has no stored chart", revision, releaseName)
	}

	upgradeClient := newRollbackClient(helmConf, target.Namespace, revision, wait, usesServerSideApply(helmConf, releaseName))
	vals := target.Config
	if vals == nil {
		vals = map[string]any{}
	}
	_, err = upgradeClient.Run(releaseName, target.Chart, vals)
	return err
}

// newRollbackClient builds the Helm upgrade action used by RunHelmRollback. It is separated from RunHelmRollback
// so its configuration can be asserted without a cluster.
func newRollbackClient(helmConf *helm.Configuration, namespace string, revision int, wait bool, serverSideApply bool) *helm.Upgrade {
	// Use only the target revision's stored user values, as helm rollback does.
	upgradeClient := newUpgradeClient(helmConf, namespace, wait, false, rollbackTimeout, serverSideApply)
	upgradeClient.Description = fmt.Sprintf("Rollback to %d", revision)
	// The chart has no rollback hooks, so helm rollback ran none. Skip the upgrade hooks too: the optional
	// pre-upgrade check rejects moving to an older version.
	upgradeClient.DisableHooks = true

	return upgradeClient
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
