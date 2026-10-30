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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
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
		require.Len(t, release.Hooks, 2, "the stub Contour chart has hooks like the certgen hooks of the Contour chart")
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
				options.Contour.ChartPath = writeStubContourChart(t, appVersion, withoutCRDs(contourBundleCRDs(), backendLBPolicies))
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
		require.ErrorContains(t, err, `helm --kube-context "test" -n radius-system rollback contour 1`)
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

	// rad install kubernetes right after k3d cluster create: the k3s helm-controller installs the Traefik CRDs while
	// Radius checks the cluster.
	t.Run("k3s installs the Traefik CRDs while Radius checks the cluster", func(t *testing.T) {
		for _, withFlag := range []bool{false, true} {
			t.Run(fmt.Sprintf("withFlag=%t", withFlag), func(t *testing.T) {
				kubeconfig, crdClient := startGatewayAPITestServer(t)
				k3s := newK3sTraefikCRDInstall(t, kubeconfig, crdClient)
				radiusChart, contourChart := writeStubCharts(t)
				looks := 0
				var firstLook error
				client := &hookedHelmClient{
					HelmClient: NewHelmClient(),
					afterGet: func(releaseName string, err error) {
						if releaseName != "traefik-crd" {
							return
						}
						looks++
						if looks == 1 {
							firstLook = err
							k3s.start()
						}
					},
					// Without the wait, k3s installs its CRDs while Radius installs Contour, as in the k3d cluster.
					beforeInstall: func(releaseName string) {
						if releaseName == contourReleaseName {
							k3s.start()
						}
					},
				}
				impl := &Impl{Helm: client}
				options := stubClusterOptions(radiusChart, contourChart)
				if withFlag {
					options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
				}

				err := impl.InstallRadius(t.Context(), options, testKubeContext)
				if withFlag {
					require.NoError(t, err)
				} else {
					// Radius decides with the CRDs of Traefik, and stops before it installs anything.
					require.True(t, clierrors.IsFriendlyError(err), "%v", err)
					require.ErrorContains(t, err, `belong to Helm release "traefik-crd" in namespace "kube-system"`)
					require.ErrorContains(t, err, turnOffFeaturesFlag)
					requireNoRelease(t, kubeconfig, contourReleaseName)
					requireNoRelease(t, kubeconfig, radiusReleaseName)
				}
				require.NoError(t, k3s.wait(t), "k3s must install its Traefik CRDs")
				require.ErrorIs(t, firstLook, driver.ErrReleaseNotFound, "Radius must look at the k3s release before k3s installs it")
				require.GreaterOrEqual(t, looks, 2, "Radius must wait until k3s installs its release")
				requireCRDsUnchanged(t, crdClient, traefikGatewayAPICRDs())
				_, err = crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(t.Context(), "ingressroutes.traefik.io", metav1.GetOptions{})
				require.NoError(t, err, "k3s must install the Traefik CRDs that are not Gateway API CRDs too")

				// The suggested flag installs Radius and Contour without changing the CRDs of Traefik.
				if !withFlag {
					options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
					require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
				}
				requireRenderedContourValues(t, getTestRelease(t, kubeconfig, contourReleaseName), "false",
					`["--disable-feature=tlsroutes","--disable-feature=tcproutes","--disable-feature=backendtlspolicies"]`)
				require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
				requireCRDsUnchanged(t, crdClient, traefikGatewayAPICRDs())
			})
		}
	})

	t.Run("failed Contour install", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}

		// Helm creates the release and then fails on a resource in a namespace that does not exist.
		brokenChart := writeStubContourChart(t, "1.32.0", contourBundleCRDs())
		writeTestFile(t, filepath.Join(brokenChart, "templates", "broken.yaml"),
			"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: broken\n  namespace: missing-namespace\n")
		err := impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, brokenChart), testKubeContext)
		require.ErrorContains(t, err, "failed to apply Contour Helm chart")
		require.Equal(t, releasecommon.StatusFailed, getTestRelease(t, kubeconfig, contourReleaseName).Info.Status)
		requireNoRelease(t, kubeconfig, radiusReleaseName)

		// Running the command again repairs the Contour release before it installs Radius.
		require.NoError(t, impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		release := getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, 2, release.Version)
		require.Equal(t, releasecommon.StatusDeployed, release.Info.Status)
		requireRenderedContourValues(t, release, "true", "[]")
		require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
	})

	// Before Radius waited for k3s, the race left a failed Contour release whose CRDs belong partly to Traefik.
	t.Run("failed Contour release that installed some of the Traefik CRDs", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		radiusChart, contourChart := writeStubCharts(t)
		impl := &Impl{Helm: NewHelmClient()}
		leftCRDs := append(withoutCRDs(contourBundleCRDs(), crdNames(traefikGatewayAPICRDs())...), traefikGatewayAPICRDs()...)
		for _, err := range createCRDs(t, crdClient, leftCRDs) {
			require.NoError(t, err)
		}
		storeFailedContourInstall(t, kubeconfig, contourChart)

		// The message lists the CRDs and how to keep them, or to install Radius without Contour.
		err := impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		require.ErrorContains(t, err, "have more than one owner")
		require.ErrorContains(t, err, "To continue without Contour, install Radius with --skip-contour-install")
		match := regexp.MustCompile(`(?m)^ +kubectl --context "test" annotate crd ([^\n]+) helm\.sh/resource-policy=keep$`).FindStringSubmatch(err.Error())
		require.Len(t, match, 2, "the message must show the kubectl command on a line of its own: %v", err)
		require.ElementsMatch(t, crdNames(contourBundleCRDs()), strings.Fields(match[1]))
		requireNoRelease(t, kubeconfig, radiusReleaseName)

		// The suggested command keeps the CRDs, and Contour then reuses them.
		for _, name := range strings.Fields(match[1]) {
			patch := []byte(`{"metadata":{"annotations":{"helm.sh/resource-policy":"keep"}}}`)
			_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Patch(t.Context(), name, types.MergePatchType, patch, metav1.PatchOptions{})
			require.NoError(t, err)
		}
		err = impl.InstallRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext)
		require.True(t, clierrors.IsFriendlyError(err), "%v", err)
		flag := regexp.MustCompile(`--contour-set ("[^"]*")`).FindStringSubmatch(err.Error())
		require.Len(t, flag, 2, "the message must suggest a --contour-set value: %v", err)
		require.Equal(t, `"contour.extraArgs={--disable-feature=backendtlspolicies}"`, flag[1])

		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{strings.Trim(flag[1], `"`)}
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
		release := getTestRelease(t, kubeconfig, contourReleaseName)
		require.Equal(t, releasecommon.StatusDeployed, release.Info.Status)
		requireRenderedContourValues(t, release, "false", `["--disable-feature=backendtlspolicies"]`)
		requireCRDsUnchanged(t, crdClient, leftCRDs)

		// Later upgrades keep reusing the CRDs.
		require.NoError(t, impl.UpgradeRadius(t.Context(), stubClusterOptions(radiusChart, contourChart), testKubeContext))
		require.Equal(t, 3, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, leftCRDs)
	})

	// v0.62.0 could install Contour, with its Gateway API CRDs, before k3s installed the Traefik CRDs. Helm then rejects
	// every attempt of k3s before it records the traefik-crd release, so Radius must not wait for that release.
	t.Run("Contour owns the Gateway API CRDs and k3s cannot install the Traefik CRDs", func(t *testing.T) {
		for _, command := range []string{"upgrade", "install --reinstall"} {
			t.Run(command, func(t *testing.T) {
				kubeconfig, crdClient := startGatewayAPITestServer(t)
				radiusChart, contourChart := writeStubCharts(t)
				options := stubClusterOptions(radiusChart, contourChart)
				require.NoError(t, (&Impl{Helm: NewHelmClient()}).InstallRadius(t.Context(), options, testKubeContext))

				k3s := newK3sTraefikCRDInstall(t, kubeconfig, crdClient)
				k3s.start()
				require.ErrorContains(t, k3s.wait(t), `key "meta.helm.sh/release-name" must equal "traefik-crd": current value is "contour"`)
				requireNoReleaseIn(t, kubeconfig, "kube-system", "traefik-crd")
				// The job of k3s restarts the failing install in the same Pod.
				createHelmControllerJob(t, kubeconfig, batchv1.JobStatus{Active: 1, StartTime: new(metav1.Now())})
				useShortK3sWait(t)

				var logs []string
				impl := &Impl{Helm: NewHelmClient(), logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }}
				switch command {
				case "upgrade":
					require.NoError(t, impl.UpgradeRadius(t.Context(), options, testKubeContext))
					require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)
				case "install --reinstall":
					// rad install kubernetes --reinstall leaves a working Contour release unchanged, so an earlier failed
					// upgrade makes it repair Contour, which waits for k3s.
					addContourRevision(t, kubeconfig, releasecommon.StatusFailed, func(*releasev1.Release) {})
					options.Radius.Reinstall = true
					require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
					release := getTestRelease(t, kubeconfig, contourReleaseName)
					require.Equal(t, 3, release.Version)
					require.Equal(t, releasecommon.StatusDeployed, release.Info.Status)
				}
				require.Equal(t, 2, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
				requireCRDsUnchanged(t, crdClient, contourBundleCRDs())
				require.Len(t, logs, 1, "%q", logs)
				require.Contains(t, logs[0], `Warning: k3s cannot install the Traefik CRDs of HelmChart kube-system/traefik-crd because CRD backendtlspolicies.gateway.networking.k8s.io belongs to Helm release "contour" in namespace "radius-system"`)
				require.Contains(t, logs[0], `kubectl --context "test" -n kube-system logs job/helm-install-traefik-crd`)
			})
		}
	})

	// helm uninstall traefik-crd keeps the CRDs of the chart, with their ownership metadata, and k3s does not run the
	// job that installed the chart again.
	t.Run("k3s installed the Traefik CRDs and their release was uninstalled", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		k3s := newK3sTraefikCRDInstall(t, kubeconfig, crdClient)
		k3s.start()
		require.NoError(t, k3s.wait(t))
		started := metav1.Now()
		createHelmControllerJob(t, kubeconfig, batchv1.JobStatus{Succeeded: 1, StartTime: &started, CompletionTime: &started})
		_, err := NewHelmClient().RunHelmUninstall(newTestHelmConfiguration(t, kubeconfig, "kube-system"), "traefik-crd", "kube-system", false)
		require.NoError(t, err)
		requireNoReleaseIn(t, kubeconfig, "kube-system", "traefik-crd")
		requireCRDsUnchanged(t, crdClient, traefikGatewayAPICRDs())
		useShortK3sWait(t)

		var logs []string
		impl := &Impl{Helm: NewHelmClient(), logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }}
		radiusChart, contourChart := writeStubCharts(t)
		options := stubClusterOptions(radiusChart, contourChart)
		options.Contour.SetArgs = []string{turnOffFeaturesForTraefikCRDs}
		require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
		requireRenderedContourValues(t, getTestRelease(t, kubeconfig, contourReleaseName), "false",
			`["--disable-feature=tlsroutes","--disable-feature=tcproutes","--disable-feature=backendtlspolicies"]`)
		requireCRDsUnchanged(t, crdClient, traefikGatewayAPICRDs())
		require.Len(t, logs, 1, "%q", logs)
		require.Contains(t, logs[0], `Warning: k3s does not install the Traefik CRDs of HelmChart kube-system/traefik-crd again: job helm-install-traefik-crd finished, but Helm release "traefik-crd" in namespace "kube-system" does not exist.`)
	})

	// A custom Contour chart can stop rendering a CRD that the Contour release installed. Radius reads the manifest of the
	// release and renders the chart as Helm's upgrade reads and renders them, so it sees that the upgrade would make Helm
	// delete the CRD, and every object of its kind.
	t.Run("custom Contour chart that stops rendering a CRD", func(t *testing.T) {
		tests := []struct {
			name string
			// template renders the HTTPProxy CRD when Radius installs Contour.
			template string
			// change changes the cluster after Contour installed the CRD, so that the chart no longer renders it.
			change func(t *testing.T, kubeconfig string, crdClient apiextensionsclient.Interface)
			// upgradeWithoutCRD is true when the commands after the install use a new chart that does not render the CRD.
			upgradeWithoutCRD bool
		}{
			{
				// The chart prefers the Gateway API, so it renders the HTTPProxy CRD only in clusters without it.
				name:     "capabilities",
				template: `{{- if not (.Capabilities.APIVersions.Has "gateway.networking.k8s.io/v1") }}` + "\n" + crdManifest(httpProxiesCRD()) + "{{- end }}\n",
				change: func(t *testing.T, kubeconfig string, crdClient apiextensionsclient.Interface) {
					for _, err := range createCRDs(t, crdClient, []crdInfo{newUnmanagedCRD("gatewayclasses", "v1")}) {
						require.NoError(t, err)
					}
					requireServedGroupVersion(t, kubeconfig, "gateway.networking.k8s.io/v1")
				},
			},
			{
				// The chart renders the HTTPProxy CRD only when it does not exist yet, so only when Helm installs it.
				name: "lookup",
				template: `{{- if not (lookup "apiextensions.k8s.io/v1" "CustomResourceDefinition" "" "httpproxies.projectcontour.io") }}` + "\n" +
					crdManifest(httpProxiesCRD()) + "{{- end }}\n",
			},
			{
				// The chart renders the HTTPProxy CRD in a List, whose items Helm applies, and its next version does not
				// render the CRD.
				name:              "list",
				template:          crdListManifest(httpProxiesCRD()),
				upgradeWithoutCRD: true,
			},
			{
				// The chart renders the HTTPProxy CRD only in the first revision of the release. After the failed revision 2,
				// Helm's upgrade renders revision 3.
				name:     "revision",
				template: "{{- if eq (int .Release.Revision) 1 }}\n" + crdManifest(httpProxiesCRD()) + "{{- end }}\n",
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				kubeconfig, crdClient := startGatewayAPITestServer(t)
				contourChart := writeStubContourChart(t, "1.33.0", nil)
				writeTestFile(t, filepath.Join(contourChart, "templates", "httpproxies.yaml"), tt.template)
				options := stubClusterOptions(writeStubRadiusChart(t), contourChart)
				// The cluster has no Gateway API CRDs for the Radius Contour Gateway, which this test does not need.
				impl := &Impl{Helm: NewHelmClient(), configureDefaultContourGateway: (&defaultContourGatewayCalls{}).configure}
				require.NoError(t, impl.InstallRadius(t.Context(), options, testKubeContext))
				createTestHTTPProxy(t, kubeconfig)
				if tt.change != nil {
					tt.change(t, kubeconfig, crdClient)
				}
				if tt.upgradeWithoutCRD {
					options.Contour.ChartPath = writeStubContourChart(t, "1.33.0", nil)
				}

				// An upgrade failed, so rad install kubernetes --reinstall repairs the Contour release.
				addContourRevision(t, kubeconfig, releasecommon.StatusFailed, func(*releasev1.Release) {})
				reinstall := options
				reinstall.Radius.Reinstall = true
				for _, command := range []struct {
					name string
					run  func() error
				}{
					{name: "rad install kubernetes --reinstall", run: func() error { return impl.InstallRadius(t.Context(), reinstall, testKubeContext) }},
					{name: "rad upgrade kubernetes", run: func() error { return impl.UpgradeRadius(t.Context(), options, testKubeContext) }},
				} {
					err := command.run()
					requireHTTPProxyKept(t, kubeconfig, crdClient)
					require.True(t, clierrors.IsFriendlyError(err), "%s must stop before Helm deletes the CRD: %v", command.name, err)
					require.ErrorContains(t, err, "the Contour chart no longer includes these CRDs")
					require.ErrorContains(t, err, "  - httpproxies.projectcontour.io\n")
					require.ErrorContains(t, err, `kubectl --context "test" annotate crd httpproxies.projectcontour.io helm.sh/resource-policy=keep`)
				}
				require.Equal(t, 1, getTestRelease(t, kubeconfig, radiusReleaseName).Version)
				require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)
			})
		}
	})
}

// httpProxyGVR is the resource of the HTTPProxy objects of Contour.
var httpProxyGVR = schema.GroupVersionResource{Group: "projectcontour.io", Version: "v1", Resource: "httpproxies"}

// TestCRDsInManifestMatchHelmBuild verifies that crdsInManifest finds the CRDs in lists that Helm's Kubernetes client
// builds from a manifest. Helm creates, updates, and deletes those CRDs with a release.
func TestCRDsInManifestMatchHelmBuild(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())

	kubeconfig, _ := startGatewayAPITestServer(t)
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfig
	client := kube.New(flags)
	for _, tt := range crdListManifestTests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := client.Build(strings.NewReader(tt.manifest), false)
			if tt.helmCannotBuild {
				require.Error(t, err, "Helm built %v", resources)
				return
			}
			require.NoError(t, err)

			var helmCRDs []string
			for _, resource := range resources {
				if resource.Object.GetObjectKind().GroupVersionKind().Kind == "CustomResourceDefinition" {
					helmCRDs = append(helmCRDs, resource.Name)
				}
			}
			crds, err := crdsInManifest(tt.manifest)
			require.NoError(t, err)
			require.True(t, slices.Equal(helmCRDs, crdNames(crds)), "Helm builds CRDs %v, and crdsInManifest finds %v", helmCRDs, crdNames(crds))
		})
	}
}

// httpProxiesCRD returns the HTTPProxy CRD of Contour, without ownership metadata.
func httpProxiesCRD() crdInfo {
	return crdInfo{name: "httpproxies.projectcontour.io", group: "projectcontour.io", servedVersions: []string{"v1"}}
}

// createTestHTTPProxy creates HTTPProxy radius-system/app once its CRD serves HTTPProxy objects.
func createTestHTTPProxy(t *testing.T, kubeconfig string) {
	t.Helper()

	client := newTestDynamicClient(t, kubeconfig)
	httpProxy := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "projectcontour.io/v1",
		"kind":       "HTTPProxy",
		"metadata":   map[string]any{"name": "app", "namespace": RadiusSystemNamespace},
		"spec":       map[string]any{"virtualhost": map[string]any{"fqdn": "app.example.com"}},
	}}
	require.Eventually(t, func() bool {
		_, err := client.Resource(httpProxyGVR).Namespace(RadiusSystemNamespace).Create(t.Context(), httpProxy, metav1.CreateOptions{})
		return err == nil
	}, 30*time.Second, 100*time.Millisecond)
}

// requireHTTPProxyKept verifies that neither the HTTPProxy CRD nor HTTPProxy radius-system/app is deleted or being
// deleted.
func requireHTTPProxyKept(t *testing.T, kubeconfig string, crdClient apiextensionsclient.Interface) {
	t.Helper()

	crd, crdErr := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(t.Context(), httpProxiesCRD().name, metav1.GetOptions{})
	crdKept := crdErr == nil && crd.DeletionTimestamp == nil
	httpProxy, httpProxyErr := newTestDynamicClient(t, kubeconfig).Resource(httpProxyGVR).Namespace(RadiusSystemNamespace).Get(t.Context(), "app", metav1.GetOptions{})
	httpProxyKept := httpProxyErr == nil && httpProxy.GetDeletionTimestamp() == nil
	require.True(t, crdKept && httpProxyKept, "Helm deleted the HTTPProxy CRD or object: CRD kept=%t (%v), HTTPProxy kept=%t (%v)",
		crdKept, crdErr, httpProxyKept, httpProxyErr)
}

// requireServedGroupVersion waits until the API server serves groupVersion.
func requireServedGroupVersion(t *testing.T, kubeconfig string, groupVersion string) {
	t.Helper()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := discovery.NewDiscoveryClientForConfig(config)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := client.ServerResourcesForGroupVersion(groupVersion)
		return err == nil
	}, 30*time.Second, 100*time.Millisecond)
}

func newTestDynamicClient(t *testing.T, kubeconfig string) dynamic.Interface {
	t.Helper()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)

	return client
}

// useShortK3sWait shortens the wait for k3s, so that a test fails quickly instead of waiting for minutes when Radius
// waits for k3s although it should not.
func useShortK3sWait(t *testing.T) {
	t.Helper()

	interval, timeout := k3sCRDWaitInterval, k3sCRDWaitTimeout
	k3sCRDWaitInterval, k3sCRDWaitTimeout = 100*time.Millisecond, 3*time.Second
	t.Cleanup(func() { k3sCRDWaitInterval, k3sCRDWaitTimeout = interval, timeout })
}

// createHelmControllerJob creates the job with which the k3s helm-controller installs HelmChart traefik-crd, with the
// given status. The API server of the tests runs no job controller, so the status does not change.
func createHelmControllerJob(t *testing.T, kubeconfig string, status batchv1.JobStatus) {
	t.Helper()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	created, err := client.BatchV1().Jobs("kube-system").Create(t.Context(), newHelmControllerJob(batchv1.JobStatus{}), metav1.CreateOptions{})
	require.NoError(t, err)
	created.Status = status
	_, err = client.BatchV1().Jobs("kube-system").UpdateStatus(t.Context(), created, metav1.UpdateOptions{})
	require.NoError(t, err)
}

// requireNoReleaseIn verifies that namespace has no Helm release with the given name.
func requireNoReleaseIn(t *testing.T, kubeconfig string, namespace string, releaseName string) {
	t.Helper()

	_, err := NewHelmClient().RunHelmGet(newTestHelmConfiguration(t, kubeconfig, namespace), releaseName)
	require.ErrorIs(t, err, driver.ErrReleaseNotFound)
}

// hookedHelmClient runs hooks around Helm operations to change the cluster while Radius installs.
type hookedHelmClient struct {
	HelmClient

	beforeInstall func(releaseName string)
	afterInstall  func(releaseName string)
	afterGet      func(releaseName string, err error)
}

func (c *hookedHelmClient) RunHelmGet(helmConf *helm.Configuration, releaseName string) (*releasev1.Release, error) {
	release, err := c.HelmClient.RunHelmGet(helmConf, releaseName)
	if c.afterGet != nil {
		c.afterGet(releaseName, err)
	}

	return release, err
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

// k3sTraefikCRDInstall plays the k3s helm-controller, which installs the CRDs of Traefik, including Gateway API CRDs,
// as Helm release traefik-crd in kube-system after the cluster starts.
type k3sTraefikCRDInstall struct {
	begin     chan struct{}
	beginOnce sync.Once

	// finished is closed when the install ends, and err is its result.
	finished chan struct{}
	err      error
}

// newK3sTraefikCRDInstall creates the HelmChart with which k3s installs the Traefik CRDs, and prepares the install,
// which runs when start is called. The hook Pods of other charts wait for the install, as Contour's certgen job waits
// for an image pull while k3s installs its CRDs.
func newK3sTraefikCRDInstall(t *testing.T, kubeconfig string, crdClient apiextensionsclient.Interface) *k3sTraefikCRDInstall {
	t.Helper()

	createNamespace(t, kubeconfig, "kube-system")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	_, err = crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(t.Context(),
		newTestCRD(crdInfo{name: "helmcharts.helm.cattle.io", group: "helm.cattle.io", servedVersions: []string{"v1"}}), metav1.CreateOptions{})
	require.NoError(t, err)
	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	helmChart := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "helm.cattle.io/v1",
		"kind":       "HelmChart",
		"metadata":   map[string]any{"name": "traefik-crd", "namespace": "kube-system"},
		"spec":       map[string]any{"chart": "https://%{KUBERNETES_API}%/static/charts/traefik-crd.tgz"},
	}}
	// The API server serves the HelmChart resource type shortly after the CRD is created.
	require.Eventually(t, func() bool {
		_, err := dynamicClient.Resource(helmChartGVR).Namespace("kube-system").Create(t.Context(), helmChart, metav1.CreateOptions{})
		return err == nil
	}, 30*time.Second, 100*time.Millisecond)

	traefikChart := filepath.Join(t.TempDir(), "traefik-crd")
	writeTestFile(t, filepath.Join(traefikChart, "Chart.yaml"), "apiVersion: v2\nname: traefik-crd\nversion: 0.1.0\n")
	// Like the CRDs of the k3s chart, these have the keep annotation, so Helm keeps them when it uninstalls the chart.
	writeTestFile(t, filepath.Join(traefikChart, "templates", "crds.yaml"),
		crdManifest(keptCRDs(append(unmanagedCRDs(traefikGatewayAPICRDs()), crdInfo{name: "ingressroutes.traefik.io", group: "traefik.io", servedVersions: []string{"v1alpha1"}}))...))
	configuration := newTestHelmConfiguration(t, kubeconfig, "kube-system")

	k3s := &k3sTraefikCRDInstall{begin: make(chan struct{}), finished: make(chan struct{})}
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		defer close(k3s.finished)
		select {
		case <-k3s.begin:
		case <-stop:
			k3s.err = errors.New("k3s did not start installing the Traefik CRDs")
			return
		}

		loaded, err := loader.Load(traefikChart)
		if err == nil {
			_, err = NewHelmClient().RunHelmInstall(configuration, loaded, map[string]any{}, "traefik-crd", "kube-system", false, time.Minute)
		}
		k3s.err = err
	}()
	hookPodGates.Store(kubeconfig, k3s.waitIfStarted)

	return k3s
}

// waitIfStarted waits for the install to end if it started.
func (k *k3sTraefikCRDInstall) waitIfStarted() {
	select {
	case <-k.begin:
		<-k.finished
	default:
	}
}

// start starts the install of the Traefik CRDs, unless it started already.
func (k *k3sTraefikCRDInstall) start() {
	k.beginOnce.Do(func() { close(k.begin) })
}

// wait returns the result of the install.
func (k *k3sTraefikCRDInstall) wait(t *testing.T) error {
	t.Helper()

	select {
	case <-k.finished:
		return k.err
	case <-time.After(2 * time.Minute):
		t.Fatal("k3s did not finish installing the Traefik CRDs")
		return nil
	}
}

// storeFailedContourInstall stores revision 1 of the Contour release as an install of contourChart with
// gatewayAPI.manageCRDs=true that failed, without changing the cluster.
func storeFailedContourInstall(t *testing.T, kubeconfig string, contourChart string) {
	t.Helper()

	loaded, err := loader.Load(contourChart)
	require.NoError(t, err)
	install := helm.NewInstall(&helm.Configuration{})
	install.DryRunStrategy = helm.DryRunClient
	install.ReleaseName = contourReleaseName
	install.Namespace = RadiusSystemNamespace
	rendered, err := install.RunWithContext(t.Context(), loaded, map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}})
	require.NoError(t, err)

	release, ok := rendered.(*releasev1.Release)
	require.True(t, ok)
	release.Info.Status = releasecommon.StatusFailed
	release.Info.Description = "Release \"contour\" failed: conflict occurred while applying object /gatewayclasses.gateway.networking.k8s.io"
	release.ApplyMethod = "ssa"
	// Helm created the namespace before the install failed.
	createNamespace(t, kubeconfig, RadiusSystemNamespace)
	require.NoError(t, newTestHelmConfiguration(t, kubeconfig, RadiusSystemNamespace).Releases.Create(release))
}

// createNamespace creates a namespace unless it exists.
func createNamespace(t *testing.T, kubeconfig string, name string) {
	t.Helper()

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := kubernetes.NewForConfig(config)
	require.NoError(t, err)
	_, err = client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}, metav1.CreateOptions{})
	if !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

// startGatewayAPITestServer starts a private API server and points KUBECONFIG at it, so that Radius reads the
// kubeconfig the way the CLI does and can reach only that API server.
func startGatewayAPITestServer(t *testing.T) (string, apiextensionsclient.Interface) {
	t.Helper()

	kubeconfig, coreClient := startTestAPIServer(t)
	t.Setenv("KUBECONFIG", kubeconfig)
	runHookPods(t, kubeconfig, coreClient)

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	crdClient, err := apiextensionsclient.NewForConfig(config)
	require.NoError(t, err)

	return kubeconfig, crdClient
}

// hookPodGates holds, by kubeconfig, a function that the test kubelet calls before it completes a hook Pod.
var hookPodGates sync.Map

// runHookPods completes the Helm hook Pods in the API server, as a kubelet would, because the API server of the tests
// runs no Pods.
func runHookPods(t *testing.T, kubeconfig string, client kubernetes.Interface) {
	t.Helper()

	// The test context ends before the cleanups run, so the loop stops before the API server does.
	ctx := t.Context()
	stopped := make(chan struct{})
	t.Cleanup(func() {
		<-stopped
		hookPodGates.Delete(kubeconfig)
	})
	go func() {
		defer close(stopped)
		for ctx.Err() == nil {
			pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
			if err != nil {
				pods = &corev1.PodList{}
			}
			for i := range pods.Items {
				pod := &pods.Items[i]
				if pod.Annotations["helm.sh/hook"] == "" || pod.Status.Phase == corev1.PodSucceeded {
					continue
				}
				if gate, found := hookPodGates.Load(kubeconfig); found {
					gate.(func())()
				}
				pod.Status.Phase = corev1.PodSucceeded
				_, _ = client.CoreV1().Pods(pod.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
			}

			select {
			case <-ctx.Done():
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
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
	// Like the certgen hooks of the Contour chart, these run after Helm checks and records the release, and before it
	// creates the CRDs.
	writeTestFile(t, filepath.Join(contourChart, "templates", "certgen.yaml"), `apiVersion: v1
kind: ServiceAccount
metadata:
  name: contour-certgen
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
---
apiVersion: v1
kind: Pod
metadata:
  name: contour-certgen
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "1"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
spec:
  restartPolicy: Never
  serviceAccountName: contour-certgen
  containers:
  - name: certgen
    image: ghcr.io/projectcontour/contour:v1.32.0
`)
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

	want = slices.SortedFunc(slices.Values(want), func(a, b crdInfo) int { return strings.Compare(a.name, b.name) })
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
