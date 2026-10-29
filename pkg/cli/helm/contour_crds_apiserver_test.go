//go:build !windows

/*
Copyright 2026 The Radius Authors.

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
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/kube"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	"helm.sh/helm/v4/pkg/storage/driver"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// testKubeContext is the context in the kubeconfig that startTestAPIServer writes.
const testKubeContext = "test"

// TestInstallContourWithExistingGatewayAPICRDs installs Radius and a stub Contour chart into a private API server. The
// stub chart renders Gateway API CRDs only when gatewayAPI.manageCRDs is true, like the real Contour chart.
func TestInstallContourWithExistingGatewayAPICRDs(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())

	// The --contour-set flag that the Contour checks suggest for the Gateway API CRDs of k3s Traefik.
	turnOffFeaturesFlag := "--contour-set " + QuoteFlagValue(turnOffFeaturesForTraefikCRDs)

	t.Run("k3s cluster with Traefik", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		traefikCRDs := traefikGatewayAPICRDs()
		for _, err := range createCRDs(t, crdClient, traefikCRDs) {
			require.NoError(t, err)
		}
		requireCRDsUnchanged(t, crdClient, traefikCRDs)

		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}

		// The value that Radius used to force makes Helm reject the CRDs that belong to Traefik.
		contourConfiguration := newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace)
		loaded, err := loader.Load(contourChart)
		require.NoError(t, err)
		_, err = NewHelmClient().RunHelmInstall(contourConfiguration, loaded, map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			contourReleaseName, RadiusSystemNamespace, false, time.Minute)
		require.ErrorContains(t, err, "invalid ownership metadata")
		require.ErrorContains(t, err, `key "meta.helm.sh/release-name" must equal "contour": current value is "traefik-crd"`)
		requireNoRelease(t, kubeconfig, contourReleaseName)

		// Radius reuses the CRDs, and stops before it installs anything because Contour 1.32.0 needs versions that
		// they do not serve.
		err = impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext)
		require.Error(t, err)
		require.True(t, clierrors.IsFriendlyError(err), "the Contour check must stop the install before Helm fails: %v", err)
		require.Contains(t, err.Error(), turnOffFeaturesFlag)
		requireNoRelease(t, kubeconfig, radiusReleaseName)
		requireNoRelease(t, kubeconfig, contourReleaseName)

		// The suggested values turn off the features that need the missing versions, and the install succeeds.
		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
		release := getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, false, nestedValue(release.Config, "gatewayAPI", "manageCRDs"))
		requireRenderedContourValues(t, release, "false", `["--disable-feature=tlsroutes","--disable-feature=tcproutes","--disable-feature=backendtlspolicies"]`)
		requireCRDsUnchanged(t, crdClient, traefikCRDs)

		// An upgrade without --contour-set keeps the values stored on the release.
		require.NoError(t, impl.UpgradeRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		release = getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, 2, release.Version)
		requireRenderedContourValues(t, release, "false", `["--disable-feature=tlsroutes","--disable-feature=tcproutes","--disable-feature=backendtlspolicies"]`)
		requireCRDsUnchanged(t, crdClient, traefikCRDs)

		// --reset-values discards those values, so the upgrade stops before it changes anything.
		resetOptions := stubClusterOptions(radiusChart, contourChart)
		resetOptions.ResetValues = true
		err = impl.UpgradeRadius(t.Context(), resetOptions, testKubeContext)
		require.ErrorContains(t, err, "Add this flag to the rad upgrade kubernetes command:\n      "+turnOffFeaturesFlag)
		require.ErrorContains(t, err, "you can instead run the upgrade without --reset-values")
		require.Equal(t, 2, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)

		// The suggested rad upgrade kubernetes flag makes the same upgrade succeed.
		resetOptions.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
		require.NoError(t, impl.UpgradeRadius(t.Context(), resetOptions, testKubeContext))
		release = getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, 3, release.Version)
		requireRenderedContourValues(t, release, "false", `["--disable-feature=tlsroutes","--disable-feature=tcproutes","--disable-feature=backendtlspolicies"]`)
		requireCRDsUnchanged(t, crdClient, traefikCRDs)
	})

	t.Run("cluster without Gateway API CRDs", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}

		// Contour installs its Gateway API CRDs.
		require.NoError(t, impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		release := getTestRelease(t, kubeconfig, contourReleaseName)
		requireRenderedContourValues(t, release, "true", "[]")
		contourCRDs := contourBundleCRDs()
		requireCRDsUnchanged(t, crdClient, contourCRDs)

		// An upgrade keeps managing them.
		require.NoError(t, impl.UpgradeRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		requireCRDsUnchanged(t, crdClient, contourCRDs)

		// Radius refuses to stop managing them, because Helm would delete them.
		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{"gatewayAPI.manageCRDs=false"}
		err := impl.UpgradeRadius(t.Context(), options, testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "the Contour check must stop the upgrade before Helm deletes the CRDs: %v", err)
		require.ErrorContains(t, err, "Helm would delete these CRDs")
		require.Equal(t, 2, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourCRDs)

		// After the CRDs are annotated as the error suggests, Helm keeps them when Contour stops managing them.
		for _, crd := range contourCRDs {
			patch := []byte(`{"metadata":{"annotations":{"helm.sh/resource-policy":"keep"}}}`)
			_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Patch(t.Context(), crd.name, types.MergePatchType, patch, metav1.PatchOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, impl.UpgradeRadius(t.Context(), options, testKubeContext))
		release = getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, 3, release.Version)
		requireRenderedContourValues(t, release, "false", "[]")
		requireCRDsUnchanged(t, crdClient, contourCRDs)
	})

	t.Run("k3s installs the Traefik CRDs while Radius installs", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		var traefikErrors []error
		client := &hookedHelmClient{
			HelmClient: NewHelmClient(),
			afterInstall: func(releaseName string) {
				if releaseName == radiusReleaseName {
					traefikErrors = createCRDs(t, crdClient, traefikGatewayAPICRDs())
				}
			},
		}

		// The cluster has no Gateway API CRDs when Radius checks it, so Contour installs its own before Radius, and k3s
		// cannot install the Traefik CRDs while Radius starts. Contour used to be installed after that wait and then
		// failed on the Traefik CRDs.
		require.NoError(t, (&Impl{Helm: client}).InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		require.Len(t, traefikErrors, len(traefikGatewayAPICRDs()))
		for _, err := range traefikErrors {
			require.True(t, apierrors.IsAlreadyExists(err), "the Traefik CRDs must not exist before Contour installs its own: %v", err)
		}
		requireRenderedContourValues(t, getTestRelease(t, kubeconfig, contourReleaseName), "true", "[]")
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
	})

	t.Run("k3s installs the Traefik CRDs right before Contour installs", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		client := &hookedHelmClient{
			HelmClient: NewHelmClient(),
			beforeInstall: func(releaseName string) {
				if releaseName == contourReleaseName {
					for _, err := range createCRDs(t, crdClient, traefikGatewayAPICRDs()) {
						require.NoError(t, err)
					}
				}
			},
		}

		// In the seconds between the checks and the Contour install, Helm itself rejects the CRDs that now belong to
		// Traefik. Radius is not installed yet, so the next attempt starts from the same state.
		err := (&Impl{Helm: client}).InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext)
		require.ErrorContains(t, err, "invalid ownership metadata")
		requireNoRelease(t, kubeconfig, contourReleaseName)
		requireNoRelease(t, kubeconfig, radiusReleaseName)

		impl := &Impl{Helm: NewHelmClient()}
		err = impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		require.ErrorContains(t, err, turnOffFeaturesFlag)

		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, traefikGatewayAPICRDs())
	})

	t.Run("Contour chart that no longer includes a CRD", func(t *testing.T) {
		backendLBPolicies := "backendlbpolicies." + gatewayAPIGroup
		for _, appVersion := range []string{"1.32.0", "1.33.0"} {
			t.Run("Contour "+appVersion, func(t *testing.T) {
				kubeconfig, crdClient := startGatewayAPITestServer(t)
				radiusChart := writeStubRadiusChart(t)
				impl := &Impl{Helm: NewHelmClient()}
				options := stubClusterOptions(radiusChart, writeStubContourChart(t, appVersion, contourBundleCRDs()))
				require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))

				// Helm would delete the CRD, and every object of its kind, because the release installed it.
				options.Contour.ChartPath = writeStubContourChart(t, appVersion, withoutCRD(contourBundleCRDs(), backendLBPolicies))
				err := impl.UpgradeRadius(t.Context(), options, testKubeContext)
				require.True(t, clierrors.IsFriendlyError(err), "%v", err)
				require.ErrorContains(t, err, "the Contour chart no longer includes these CRDs")
				require.ErrorContains(t, err, "  - "+backendLBPolicies+"\n")
				require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
				require.Equal(t, 1, getTestRelease(t, kubeconfig, contourReleaseName).Version)
				requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
			})
		}
	})

	t.Run("failed Contour revision without the CRDs", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}
		require.NoError(t, impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))

		// Revision 2 failed after it stopped rendering the CRDs, so its manifest lacks them. Helm upgrades from revision
		// 1, which is still deployed, and deletes the CRDs in its manifest that the chart no longer renders.
		addContourRevision(t, kubeconfig, releasecommon.StatusFailed, func(revision *releasev1.Release) {
			revision.Manifest = withoutCRDDocuments(revision.Manifest)
			revision.Config = map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}}
		})

		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{"gatewayAPI.manageCRDs=false"}
		err := impl.UpgradeRadius(t.Context(), options, testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		require.ErrorContains(t, err, "Helm would delete these CRDs")
		for _, crd := range contourBundleCRDs() {
			require.ErrorContains(t, err, "  - "+crd.name+"\n")
		}
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
	})

	t.Run("pending Contour revision", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}
		options := stubClusterOptions(radiusChart, contourChart)
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))

		// An interrupted upgrade left revision 2 pending, so Helm would refuse to upgrade Contour after Radius.
		addContourRevision(t, kubeconfig, releasecommon.StatusPendingUpgrade, func(*releasev1.Release) {})
		_, err := impl.CheckContourUpgrade(t.Context(), options, testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		err = impl.UpgradeRadius(t.Context(), options, testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		require.ErrorContains(t, err, "revision 2 is pending-upgrade")
		require.ErrorContains(t, err, "helm -n radius-system rollback contour 1")
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)

		// The rollback that the error suggests makes the upgrade succeed.
		rollback := helm.NewRollback(newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace))
		rollback.Version = 1
		rollback.WaitStrategy = kube.HookOnlyStrategy
		require.NoError(t, rollback.Run(contourReleaseName))
		require.NoError(t, impl.UpgradeRadius(t.Context(), options, testKubeContext))
		require.Equal(t, 2, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		require.Equal(t, 4, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
	})

	t.Run("Contour chart that removes a stored CRD version", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart := writeStubRadiusChart(t)
		impl := &Impl{Helm: NewHelmClient()}
		options := stubClusterOptions(radiusChart, writeStubContourChart(t, "1.33.0", contourBundleCRDs()))
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))

		udpRoutes := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "udproutes", "v1")
		options.Contour.ChartPath = writeStubContourChart(t, "1.33.0", replaceCRD(contourBundleCRDs(), udpRoutes))
		err := impl.UpgradeRadius(t.Context(), options, testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		require.ErrorContains(t, err, "  - udproutes.gateway.networking.k8s.io stores objects in v1alpha2, but the chart's definition has v1\n")
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		require.Equal(t, 1, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourBundleCRDs())

		// Without the check, the API server rejects the CRD while Helm applies the chart.
		loaded, err := loader.Load(options.Contour.ChartPath)
		require.NoError(t, err)
		_, err = NewHelmClient().RunHelmUpgrade(newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace), loaded, map[string]any{},
			contourReleaseName, RadiusSystemNamespace, false, true, time.Minute)
		require.ErrorContains(t, err, "must appear in spec.versions")
	})
}

// hookedHelmClient runs hooks around Helm installs to change the cluster while Radius installs.
type hookedHelmClient struct {
	HelmClient

	beforeInstall func(releaseName string)
	afterInstall  func(releaseName string)
}

func (c *hookedHelmClient) RunHelmInstall(helmConf *helm.Configuration, helmChart *chart.Chart, vals map[string]any, releaseName, namespace string, wait bool, timeout time.Duration) (*releasev1.Release, error) {
	if c.beforeInstall != nil {
		c.beforeInstall(releaseName)
	}
	release, err := c.HelmClient.RunHelmInstall(helmConf, helmChart, vals, releaseName, namespace, wait, timeout)
	if c.afterInstall != nil {
		c.afterInstall(releaseName)
	}

	return release, err
}

// createCRDs creates crds in the API server and returns the error of each create.
func createCRDs(t *testing.T, crdClient apiextensionsclient.Interface, crds []crdInfo) []error {
	t.Helper()

	errs := make([]error, 0, len(crds))
	for _, crd := range crds {
		_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(t.Context(), newTestCRD(crd), metav1.CreateOptions{})
		errs = append(errs, err)
	}

	return errs
}

// addContourRevision stores a revision of the Contour release after the latest one, as a Helm operation that failed or
// did not finish leaves it. change edits the revision before it is stored.
func addContourRevision(t *testing.T, kubeconfig string, status releasecommon.Status, change func(*releasev1.Release)) {
	t.Helper()

	latest := getTestRelease(t, kubeconfig, contourReleaseName)
	revision := *latest
	info := *latest.Info
	info.Status = status
	revision.Info = &info
	revision.Version = latest.Version + 1
	change(&revision)
	require.NoError(t, newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace).Releases.Create(&revision))
}

// withoutCRDDocuments returns manifest without its CRDs.
func withoutCRDDocuments(manifest string) string {
	var documents []string
	for _, document := range releaseutil.SplitManifests(manifest) {
		if !strings.Contains(document, "kind: CustomResourceDefinition") {
			documents = append(documents, document)
		}
	}
	slices.Sort(documents)

	return "---\n" + strings.Join(documents, "\n---\n")
}

// withoutCRD returns crds without the CRD with the given name.
func withoutCRD(crds []crdInfo, name string) []crdInfo {
	return slices.DeleteFunc(slices.Clone(crds), func(crd crdInfo) bool { return crd.name == name })
}

// startGatewayAPITestServer starts a private API server and points KUBECONFIG at it, so that Radius reads the
// kubeconfig the way the CLI does and can reach only that API server.
func startGatewayAPITestServer(t *testing.T) (string, apiextensionsclient.Interface) {
	t.Helper()

	kubeconfig, _ := startTestAPIServer(t)
	t.Setenv("KUBECONFIG", kubeconfig)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	crdClient, err := apiextensionsclient.NewForConfig(config)
	require.NoError(t, err)

	return kubeconfig, crdClient
}

// writeStubCharts writes a Radius chart, and a Contour chart that renders the Gateway API CRDs that Contour 1.32.0
// bundles.
func writeStubCharts(t *testing.T) (string, string) {
	t.Helper()

	return writeStubRadiusChart(t), writeStubContourChart(t, "1.32.0", contourBundleCRDs())
}

// writeStubRadiusChart writes a Radius chart that renders a ConfigMap.
func writeStubRadiusChart(t *testing.T) string {
	t.Helper()

	radiusChart := filepath.Join(t.TempDir(), "radius")
	writeTestFile(t, filepath.Join(radiusChart, "Chart.yaml"), "apiVersion: v2\nname: radius\nversion: 0.1.0\n")
	writeTestFile(t, filepath.Join(radiusChart, "templates", "configmap.yaml"),
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: radius-stub\ndata:\n  installed: \"true\"\n")

	return radiusChart
}

// writeStubContourChart writes a Contour chart that deploys the given Contour version and renders a ConfigMap. Like
// the real Contour chart, it renders gatewayCRDs only when gatewayAPI.manageCRDs is true.
func writeStubContourChart(t *testing.T, appVersion string, gatewayCRDs []crdInfo) string {
	t.Helper()

	contourChart := filepath.Join(t.TempDir(), "contour")
	writeTestFile(t, filepath.Join(contourChart, "Chart.yaml"), "apiVersion: v2\nname: contour\nversion: 0.1.0\nappVersion: "+appVersion+"\n")
	writeTestFile(t, filepath.Join(contourChart, "values.yaml"), "contour:\n  extraArgs: []\ngatewayAPI:\n  manageCRDs: false\n")
	writeTestFile(t, filepath.Join(contourChart, "templates", "crds", "gateway-api-crds.yaml"), gatedCRDTemplate(gatewayCRDs))
	writeTestFile(t, filepath.Join(contourChart, "templates", "configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: contour-stub
data:
  manageCRDs: {{ .Values.gatewayAPI.manageCRDs | quote }}
  extraArgs: {{ .Values.contour.extraArgs | toJson | quote }}
`)

	return contourChart
}

func stubClusterOptions(radiusChart string, contourChart string) ClusterOptions {
	options := NewDefaultClusterOptions()
	options.Radius.ChartPath = radiusChart
	options.Radius.Timeout = time.Minute
	options.Contour.ChartPath = contourChart
	options.Contour.Timeout = time.Minute
	return options
}

// requireCRDsUnchanged verifies that the cluster has exactly the Gateway API CRDs in want, with the same owners and
// served versions.
func requireCRDsUnchanged(t *testing.T, crdClient apiextensionsclient.Interface, want []crdInfo) {
	t.Helper()

	list, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	var names []string
	for _, item := range list.Items {
		if (crdInfo{group: item.Spec.Group}).isGatewayAPI() {
			names = append(names, item.Name)
		}
	}
	got, err := getCRDsWithClient(t.Context(), crdClient, names)
	require.NoError(t, err)
	require.Len(t, got, len(want))
	for i := range want {
		require.Equal(t, want[i].name, got[i].name)
		require.Equal(t, want[i].servedVersions, got[i].servedVersions, got[i].name)
		for _, key := range []string{helmManagedByLabel} {
			require.Equal(t, want[i].labels[key], got[i].labels[key], got[i].name)
		}
		for _, key := range []string{helmReleaseNameAnnotation, helmReleaseNamespaceAnnotation} {
			require.Equal(t, want[i].annotations[key], got[i].annotations[key], got[i].name)
		}
	}
}

// requireRenderedContourValues verifies the Contour values that the stub chart rendered.
func requireRenderedContourValues(t *testing.T, release *releasev1.Release, manageCRDs string, extraArgs string) {
	t.Helper()

	// The stub chart renders the values with quote, which formats them like strconv.Quote.
	require.Contains(t, release.Manifest, "manageCRDs: "+strconv.Quote(manageCRDs))
	require.Contains(t, release.Manifest, "extraArgs: "+strconv.Quote(extraArgs))
}

func newTestHelmConfiguration(t *testing.T, kubeconfig string, namespace string) *helm.Configuration {
	t.Helper()

	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfig
	flags.Namespace = &namespace
	configuration := &helm.Configuration{}
	require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))

	return configuration
}

func getTestRelease(t *testing.T, kubeconfig string, releaseName string) *releasev1.Release {
	t.Helper()

	release, err := NewHelmClient().RunHelmGet(newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace), releaseName)
	require.NoError(t, err)

	return release
}

func requireNoRelease(t *testing.T, kubeconfig string, releaseName string) {
	t.Helper()

	_, err := NewHelmClient().RunHelmGet(newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace), releaseName)
	require.ErrorIs(t, err, driver.ErrReleaseNotFound)
}

func writeTestFile(t *testing.T, path string, contents string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
	require.NoError(t, os.WriteFile(path, []byte(contents), 0600))
}
