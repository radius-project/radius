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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/common"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/kube"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
	"helm.sh/helm/v4/pkg/strvals"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	k8stesting "k8s.io/client-go/testing"
	"sigs.k8s.io/yaml"
)

// contourChartAppVersions maps each Contour chart version that Radius installs by default to the Contour version that
// the chart deploys, which is its appVersion.
var contourChartAppVersions = map[string]string{
	"0.1.0": "1.32.0",
}

// Test_ContourChartDefaultVersion_HasGatewayAPIRequirements fails when the default Contour chart changes without a
// review of the Gateway API resources that the new Contour version watches.
func Test_ContourChartDefaultVersion_HasGatewayAPIRequirements(t *testing.T) {
	t.Parallel()

	appVersion, found := contourChartAppVersions[ContourChartDefaultVersion]
	require.Truef(t, found, "add Contour chart %s and its appVersion to contourChartAppVersions", ContourChartDefaultVersion)

	requirements, found := contourGatewayAPIRequirements[appVersion]
	require.Truef(t, found, "add the Gateway API resources that Contour %s watches to contourGatewayAPIRequirements", appVersion)
	require.NotEmpty(t, requirements)
	for _, requirement := range requirements {
		require.True(t, strings.HasSuffix(requirement.crd, "."+gatewayAPIGroup), "%s is not a Gateway API CRD", requirement.crd)
	}
}

func Test_gatewayAPICRDCheck(t *testing.T) {
	t.Parallel()

	turnOffUnsupportedFeatures := map[string]any{
		"contour": map[string]any{
			"extraArgs": []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
		},
	}
	traefikWithoutHTTPRoutes := []crdInfo{}
	for _, crd := range traefikGatewayAPICRDs() {
		if crd.name != "httproutes."+gatewayAPIGroup {
			traefikWithoutHTTPRoutes = append(traefikWithoutHTTPRoutes, crd)
		}
	}
	listenerSets := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "listenersets", "v1")
	keptListenerSets := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "listenersets", "v1")
	keptListenerSets.annotations[kube.ResourcePolicyAnno] = kube.KeepPolicy
	httpProxies := crdInfo{
		name:           "httpproxies.projectcontour.io",
		group:          "projectcontour.io",
		servedVersions: []string{"v1"},
		labels:         map[string]string{helmManagedByLabel: helmManagedByValue},
		annotations:    map[string]string{helmReleaseNameAnnotation: contourReleaseName, helmReleaseNamespaceAnnotation: RadiusSystemNamespace},
	}
	udpRoutes := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "udproutes", "v1alpha2")
	experimentalListenerSets := newHelmOwnedCRD("traefik-crd", "kube-system", "xlistenersets", "v1alpha1")
	experimentalListenerSets.name = "xlistenersets." + gatewayAPIExperimentalGroup
	experimentalListenerSets.group = gatewayAPIExperimentalGroup

	// chartWithCRDDirectory renders the Contour 1.32.0 bundle like the Contour chart, and also has a crds/ directory
	// with TLSRoute and TCPRoute CRDs, and a BackendTLSPolicy CRD that serves v1alpha3.
	chartWithCRDDirectory := newTestContourChart()
	for _, crd := range []crdInfo{
		newUnmanagedCRD("tlsroutes", "v1alpha2"),
		newUnmanagedCRD("tcproutes", "v1alpha2"),
		newUnmanagedCRD("backendtlspolicies", "v1alpha3"),
	} {
		chartWithCRDDirectory.Files = append(chartWithCRDDirectory.Files, &common.File{Name: "crds/" + crd.name + ".yaml", Data: []byte(crdManifest(crd))})
	}

	tests := []struct {
		name  string
		chart *chart.Chart
		// capabilities are the capabilities of the cluster, or nil for Helm's default capabilities.
		capabilities    *common.Capabilities
		crds            []crdInfo
		release         []crdInfo
		upgrade         bool
		stored          map[string]any
		resetValues     bool
		command         string
		values          map[string]any
		wantManageCRDs  any
		wantErr         []string
		wantNotErr      []string
		wantRenderError bool
		wantWarning     bool
	}{
		{
			name:           "fresh cluster",
			wantManageCRDs: true,
		},
		{
			name:           "CRDs installed by the Contour release",
			crds:           contourBundleCRDs(),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
		},
		{
			name:           "CRDs that Helm can adopt into the Contour release",
			crds:           contourBundleCRDs(),
			wantManageCRDs: true,
		},
		{
			name:           "CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			wantManageCRDs: false,
			wantErr: []string{
				"Radius stopped before changing the cluster because Contour 1.32.0 cannot use the Gateway API CRDs in this cluster.",
				"tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)",
				"tcproutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)",
				"backendtlspolicies.gateway.networking.k8s.io v1alpha3 (the installed CRD serves v1)",
				`belong to Helm release "traefik-crd" in namespace "kube-system", so Contour uses them instead of installing its own.`,
				"Contour then ignores TLSRoute, TCPRoute, and BackendTLSPolicy resources.",
				"Add this flag to the rad install kubernetes command:",
				"--contour-set \"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}\"",
				"If you used rad init, run rad install kubernetes with this flag first, and then run rad init again.",
				`--k3s-arg "--disable=traefik@server:*"`,
			},
			wantNotErr: []string{"grpcroutes", "gatewayclasses", "Contour values set gatewayAPI.manageCRDs", "Remove the Contour value", "--reset-values"},
		},
		{
			name:           "CRDs from the k3s Traefik chart with the unsupported features turned off",
			crds:           traefikGatewayAPICRDs(),
			values:         turnOffUnsupportedFeatures,
			wantManageCRDs: false,
		},
		{
			name: "CRDs from the k3s Traefik chart with features turned off by separate arguments",
			crds: traefikGatewayAPICRDs(),
			values: map[string]any{
				"contour": map[string]any{
					"extraArgs": []any{"--disable-feature", "tlsroutes", "--disable-feature", "tcproutes", "--disable-feature", "backendtlspolicies"},
				},
			},
			wantManageCRDs: false,
		},
		{
			name: "CRDs from the k3s Traefik chart with features turned off by a YAML string",
			crds: traefikGatewayAPICRDs(),
			values: map[string]any{
				"contour": map[string]any{
					"extraArgs": "- --disable-feature=tlsroutes\n- --disable-feature=tcproutes\n- --disable-feature=backendtlspolicies\n",
				},
			},
			wantManageCRDs: false,
		},
		{
			name: "contour.args as a YAML string keeps its arguments in the suggestion",
			crds: traefikGatewayAPICRDs(),
			values: map[string]any{
				"contour": map[string]any{"args": "- serve\n- --incluster\n- --disable-feature=tlsroutes\n"},
			},
			wantManageCRDs: false,
			wantErr: []string{
				"--contour-set \"contour.args={serve,--incluster,--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}\"",
			},
			wantNotErr: []string{"tlsroutes.gateway.networking.k8s.io v1alpha2"},
		},
		{
			name: "contour.extraArgs that is not a list",
			crds: traefikGatewayAPICRDs(),
			values: map[string]any{
				"contour": map[string]any{"extraArgs": "--disable-feature=tlsroutes"},
			},
			wantManageCRDs: false,
			wantErr: []string{
				"Radius cannot read your contour.extraArgs value as a list, so add these arguments to it:\n      --disable-feature=tlsroutes\n      --disable-feature=tcproutes\n      --disable-feature=backendtlspolicies",
			},
			wantNotErr: []string{"--contour-set", "rad install kubernetes with this flag first"},
		},
		{
			name: "contour.args that is not a list",
			crds: traefikGatewayAPICRDs(),
			values: map[string]any{
				"contour": map[string]any{"args": "serve --incluster"},
			},
			wantManageCRDs: false,
			wantErr:        []string{"Radius cannot read your contour.args value as a list, so add these arguments to it:"},
			wantNotErr:     []string{"--contour-set", "contour.args={"},
		},
		{
			name:           "upgrade with CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			wantManageCRDs: false,
			wantErr: []string{
				"Add this flag to the rad upgrade kubernetes command:",
				"--contour-set \"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}\"",
			},
			wantNotErr: []string{"--reset-values", "rad init", "rad install kubernetes"},
		},
		{
			name:           "upgrade that reuses the values that turn off the unsupported features",
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			stored:         turnOffUnsupportedFeatures,
			wantManageCRDs: false,
		},
		{
			name:           "upgrade with --reset-values and CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			stored:         turnOffUnsupportedFeatures,
			resetValues:    true,
			wantManageCRDs: false,
			wantErr: []string{
				"Add this flag to the rad upgrade kubernetes command:",
				"If you set this value when you installed Radius, you can instead run the upgrade without --reset-values.",
			},
		},
		{
			name:           "compatible CRDs installed without Helm",
			crds:           unmanagedCRDs(contourBundleCRDs()),
			wantManageCRDs: false,
		},
		{
			name:           "missing CRD that Contour always watches",
			crds:           traefikWithoutHTTPRoutes,
			values:         turnOffUnsupportedFeatures,
			wantManageCRDs: false,
			wantErr:        []string{"httproutes.gateway.networking.k8s.io v1 (the CRD is not installed)"},
			wantNotErr:     []string{"Turn off the Contour features"},
		},
		{
			// Helm never deletes CRDs that are not in the release manifest, so Contour reuses the CRDs of the other owner.
			name: "CRDs that only carry the ownership metadata of the Contour release, and CRDs that belong to something else",
			crds: append(contourBundleCRDs()[:2],
				newHelmOwnedCRD("traefik-crd", "kube-system", "gateways", "v1")),
			wantManageCRDs: false,
			wantErr:        []string{"gatewayclasses.gateway.networking.k8s.io v1 (the CRD is not installed)"},
			wantNotErr:     []string{"more than one owner", "backendtlspolicies"},
		},
		{
			// k3s installed its Traefik CRDs while Contour installed its own, so the failed release installed some CRDs
			// and k3s took over the others.
			name:           "CRDs installed by the Contour release, some of which another release took over",
			crds:           append(withoutCRDs(contourBundleCRDs(), crdNames(traefikGatewayAPICRDs())...), traefikGatewayAPICRDs()...),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
			wantErr: []string{
				`have more than one owner. Contour release "contour" in namespace "radius-system" installed these CRDs, so Helm would delete them if Contour stopped managing them:` + "\n" +
					"  - backendlbpolicies.gateway.networking.k8s.io\n" +
					`  - backendtlspolicies.gateway.networking.k8s.io (now owned by Helm release "traefik-crd" in namespace "kube-system")` + "\n",
				"  - udproutes.gateway.networking.k8s.io\n",
				"Radius neither takes over CRDs that belong to something else nor lets Helm delete the CRDs that the Contour release installed.",
				`kubectl --context "test-context" annotate crd backendlbpolicies.gateway.networking.k8s.io backendtlspolicies.gateway.networking.k8s.io gatewayclasses.gateway.networking.k8s.io`,
				"udproutes.gateway.networking.k8s.io helm.sh/resource-policy=keep",
			},
			wantNotErr: []string{"These CRDs belong to something else", "uninstall"},
		},
		{
			// The remedy of the case above: with all of these CRDs kept, Contour reuses them.
			name:           "CRDs installed by the Contour release and kept, some of which another release took over",
			crds:           keptCRDs(append(withoutCRDs(contourBundleCRDs(), crdNames(traefikGatewayAPICRDs())...), traefikGatewayAPICRDs()...)),
			release:        contourBundleCRDs(),
			wantManageCRDs: false,
			wantErr: []string{
				"backendtlspolicies.gateway.networking.k8s.io v1alpha3 (the installed CRD serves v1)",
				`--contour-set "contour.extraArgs={--disable-feature=backendtlspolicies}"`,
			},
			wantNotErr: []string{"tlsroutes", "tcproutes", "more than one owner", "Helm would delete"},
		},
		{
			name:           "CRDs installed by the Contour release, all of which another release took over",
			crds:           traefikGatewayAPICRDs(),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
			wantErr: []string{
				"have more than one owner",
				`  - gateways.gateway.networking.k8s.io (now owned by Helm release "traefik-crd" in namespace "kube-system")` + "\n",
				`kubectl --context "test-context" annotate crd backendtlspolicies.gateway.networking.k8s.io gatewayclasses.gateway.networking.k8s.io`,
			},
		},
		{
			name:           "CRDs installed by the Contour release, and a CRD of another owner that a new chart installs",
			chart:          newStubContourChart("1.32.0", append(contourBundleCRDs(), experimentalListenerSets)),
			crds:           append(contourBundleCRDs(), experimentalListenerSets),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
			wantErr: []string{
				"  - backendlbpolicies.gateway.networking.k8s.io\n",
				"These CRDs belong to something else:\n" +
					`  - xlistenersets.gateway.networking.x-k8s.io (owned by Helm release "traefik-crd" in namespace "kube-system")` + "\n",
			},
			wantNotErr: []string{"now owned by"},
		},
		{
			name:           "foreign CRD that the chart does not install",
			crds:           []crdInfo{newUnmanagedCRD("listenersets", "v1")},
			wantManageCRDs: true,
		},
		{
			name:           "CRDs installed by the Contour release and a foreign CRD that the chart does not install",
			crds:           append(contourBundleCRDs(), newUnmanagedCRD("listenersets", "v1")),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
		},
		{
			name:           "Gateway API CRDs of the experimental group",
			chart:          newStubContourChart("1.32.0", append(contourBundleCRDs(), experimentalListenerSets)),
			crds:           []crdInfo{experimentalListenerSets},
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantManageCRDs: true,
			wantErr:        []string{`xlistenersets.gateway.networking.x-k8s.io (owned by Helm release "traefik-crd" in namespace "kube-system")`},
		},
		{
			name:           "gatewayAPI.manageCRDs=true set by the user with foreign CRDs",
			crds:           traefikGatewayAPICRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantManageCRDs: true,
			wantErr: []string{
				"these Gateway API CRDs already exist and belong to something else",
				`gatewayclasses.gateway.networking.k8s.io (owned by Helm release "traefik-crd" in namespace "kube-system")`,
				"Remove the gatewayAPI.manageCRDs=true Contour value",
			},
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user with CRDs installed by the Contour release",
			crds:           contourBundleCRDs(),
			release:        contourBundleCRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr: []string{
				`the Contour chart no longer includes these CRDs, which Contour release "contour" in namespace "radius-system" installed:`,
				"  - gateways.gateway.networking.k8s.io\n",
				"Helm would delete these CRDs, and every resource of these kinds in the cluster with them.",
				"  - Remove the Contour value that sets gatewayAPI.manageCRDs to false.",
				`kubectl --context "test-context" annotate crd backendlbpolicies.gateway.networking.k8s.io backendtlspolicies.gateway.networking.k8s.io`,
				"udproutes.gateway.networking.k8s.io helm.sh/resource-policy=keep",
			},
			wantNotErr: []string{"Change the Contour chart"},
		},
		{
			name:           "a CRD that the release installed and the chart no longer renders",
			crds:           append(contourBundleCRDs(), listenerSets),
			release:        append(contourBundleCRDs(), listenerSets),
			wantManageCRDs: true,
			wantErr: []string{
				"the Contour chart no longer includes these CRDs",
				"  - listenersets.gateway.networking.k8s.io\n",
				"  - Change the Contour chart or values so that the chart still includes these CRDs.",
				`kubectl --context "test-context" annotate crd listenersets.gateway.networking.k8s.io helm.sh/resource-policy=keep`,
			},
			wantNotErr: []string{"gateways.gateway.networking.k8s.io", "Remove the Contour value"},
		},
		{
			name:           "a CRD that the chart renders only when Helm installs it",
			chart:          newInstallOnlyCRDChart(),
			crds:           []crdInfo{listenerSets},
			release:        []crdInfo{listenerSets},
			values:         turnOffUnsupportedFeatures,
			wantManageCRDs: true,
			wantErr:        []string{"the Contour chart no longer includes these CRDs", "  - listenersets.gateway.networking.k8s.io\n"},
		},
		{
			// Radius renders the chart with the capabilities of the cluster, as Helm's upgrade does. Helm's default
			// capabilities lack Gateway API v1, so a render with them would include the CRD that Helm then deletes.
			name:           "a CRD that the chart renders only without Gateway API v1, in a cluster that serves it",
			chart:          newCapabilitiesGatedCRDChart(),
			capabilities:   capabilitiesWith("gateway.networking.k8s.io/v1"),
			crds:           []crdInfo{httpProxies},
			release:        []crdInfo{httpProxies},
			wantManageCRDs: true,
			wantErr: []string{
				"the Contour chart no longer includes these CRDs",
				"  - httpproxies.projectcontour.io\n",
				`kubectl --context "test-context" annotate crd httpproxies.projectcontour.io helm.sh/resource-policy=keep`,
			},
		},
		{
			name:           "a CRD that the chart renders only without Gateway API v1, in a cluster that does not serve it",
			chart:          newCapabilitiesGatedCRDChart(),
			crds:           []crdInfo{httpProxies},
			release:        []crdInfo{httpProxies},
			wantManageCRDs: true,
			wantWarning:    true,
		},
		{
			name:           "a CRD that the release installed and the chart no longer renders, with the keep annotation",
			crds:           append(contourBundleCRDs(), keptListenerSets),
			release:        append(contourBundleCRDs(), listenerSets),
			wantManageCRDs: true,
		},
		{
			name:           "a CRD that the release installed and that no longer exists",
			crds:           contourBundleCRDs(),
			release:        append(contourBundleCRDs(), listenerSets),
			wantManageCRDs: true,
		},
		{
			name:           "a CRD that the release installed and that another release took over",
			crds:           append(contourBundleCRDs(), newHelmOwnedCRD("traefik-crd", "kube-system", "listenersets", "v1")),
			release:        append(contourBundleCRDs(), listenerSets),
			wantManageCRDs: true,
			wantErr:        []string{`  - listenersets.gateway.networking.k8s.io (now owned by Helm release "traefik-crd" in namespace "kube-system")`},
		},
		{
			name:           "a Contour CRD that the release installed and the chart no longer renders",
			crds:           append(contourBundleCRDs(), httpProxies),
			release:        append(contourBundleCRDs(), httpProxies),
			wantManageCRDs: true,
			wantErr:        []string{"  - httpproxies.projectcontour.io\n", "Change the Contour chart or values"},
		},
		{
			name:           "a CRD that the chart updates without a version that the cluster stores objects in",
			chart:          newStubContourChart("1.32.0", replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1"}))),
			crds:           replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1alpha2"}, "v1alpha2")),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
			wantErr: []string{
				"the Contour chart removes versions of these CRDs that the cluster stores objects in:\n" +
					"  - udproutes.gateway.networking.k8s.io stores objects in v1alpha2, but the chart's definition has v1\n",
				"The Kubernetes API server rejects a CRD update that removes a stored version",
				"  - Use a Contour chart whose CRDs keep these versions.",
				"remove the old versions from status.storedVersions of these CRDs",
			},
		},
		{
			name:           "a CRD that Helm can adopt, and that the chart defines without a version that the cluster stores objects in",
			chart:          newStubContourChart("1.32.0", replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1"}))),
			crds:           replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1alpha2"}, "v1alpha2")),
			wantManageCRDs: true,
			wantErr:        []string{"  - udproutes.gateway.networking.k8s.io stores objects in v1alpha2, but the chart's definition has v1\n"},
		},
		{
			name:           "a CRD that the chart updates to stop serving a version that the cluster stores objects in",
			chart:          newStubContourChart("1.32.0", replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1", "v1alpha2"}))),
			crds:           replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1alpha2"}, "v1alpha2")),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
		},
		{
			name:           "a CRD that the chart creates",
			chart:          newStubContourChart("1.32.0", replaceCRD(contourBundleCRDs(), storedIn(udpRoutes, []string{"v1"}))),
			wantManageCRDs: true,
		}, {
			name:           "gatewayAPI.manageCRDs=false set by the user with adoptable CRDs that no release installed",
			crds:           contourBundleCRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user on a fresh cluster",
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr: []string{
				"Contour does not install Gateway API CRDs because the Contour values set gatewayAPI.manageCRDs to false.\nTo continue",
				"\n  - Remove the Contour value that sets gatewayAPI.manageCRDs to false, so that Contour installs the Gateway API CRDs that it needs.",
				"\n  - Install Gateway API CRDs that serve these versions.",
			},
			wantNotErr: []string{"k3s-arg", "instead of installing its own", "belong to", "Turn off the Contour features"},
		},
		{
			name:            "gatewayAPI set to null by the user, which the chart cannot render",
			values:          map[string]any{"gatewayAPI": nil},
			wantManageCRDs:  nil,
			wantRenderError: true,
			wantErr:         []string{"failed to render the Contour Helm chart"},
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user with CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr: []string{
				"the Contour values set gatewayAPI.manageCRDs to false.",
				`The Gateway API CRDs in this cluster belong to Helm release "traefik-crd" in namespace "kube-system".`,
				"--contour-set \"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}\"",
			},
			// Contour would take over these CRDs if it managed them, so removing the value does not help.
			wantNotErr: []string{"Remove the Contour value", "k3s-arg", "instead of installing its own"},
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user with a foreign CRD that neither the chart nor Contour uses",
			crds:           []crdInfo{newUnmanagedCRD("listenersets", "v1")},
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr:        []string{"Remove the Contour value that sets gatewayAPI.manageCRDs to false"},
			wantNotErr:     []string{"k3s-arg", "listenersets", "belong to"},
		},
		{
			name:           "a chart whose Gateway API CRDs do not serve the versions that Contour needs",
			chart:          newStubContourChart("1.32.0", unmanagedCRDs(traefikGatewayAPICRDs())),
			wantManageCRDs: true,
			wantErr: []string{
				"tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)",
				"backendtlspolicies.gateway.networking.k8s.io v1alpha3 (the CRD that the Contour chart installs serves v1)",
				"The Contour chart does not install Gateway API CRDs that serve all of these versions.",
				"  - Use a Contour chart that installs Gateway API CRDs that serve these versions.",
				"--contour-set \"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}\"",
			},
			wantNotErr: []string{"k3s-arg", "Install Gateway API CRDs"},
		},
		{
			name:           "CRDs in the chart's crds/ directory that Helm creates on install",
			chart:          chartWithCRDDirectory,
			crds:           traefikGatewayAPICRDs(),
			wantManageCRDs: false,
			wantErr:        []string{"backendtlspolicies.gateway.networking.k8s.io v1alpha3 (the installed CRD serves v1)"},
			wantNotErr:     []string{"tlsroutes", "tcproutes"},
		},
		{
			name:           "CRDs in the chart's crds/ directory that Helm ignores on upgrade",
			chart:          chartWithCRDDirectory,
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			wantManageCRDs: false,
			wantErr:        []string{"tlsroutes.gateway.networking.k8s.io v1alpha2 (the CRD is not installed)"},
		},
		{
			name:           "unknown Contour version with foreign CRDs",
			chart:          newStubContourChart("9.9.9", contourBundleCRDs()),
			crds:           traefikGatewayAPICRDs(),
			wantManageCRDs: false,
			wantWarning:    true,
		},
		{
			name:           "unknown Contour version uses the CRDs that the chart renders",
			chart:          newStubContourChart("9.9.9", contourBundleCRDs()),
			crds:           []crdInfo{newUnmanagedCRD("listenersets", "v1")},
			wantManageCRDs: true,
			wantWarning:    true,
		},
		{
			name:           "unknown Contour version with gatewayAPI.manageCRDs=true set by the user and foreign CRDs",
			chart:          newStubContourChart("9.9.9", contourBundleCRDs()),
			crds:           traefikGatewayAPICRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantManageCRDs: true,
			wantErr:        []string{"these Gateway API CRDs already exist and belong to something else"},
		},
		{
			name:           "chart without appVersion",
			chart:          newStubContourChart("", contourBundleCRDs()),
			wantManageCRDs: true,
			wantWarning:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helmChart := tt.chart
			if helmChart == nil {
				helmChart = newTestContourChart()
			}
			var baseline *releasev1.Release
			if tt.release != nil || tt.upgrade {
				baseline = &releasev1.Release{
					Name:     contourReleaseName,
					Version:  1,
					Info:     &releasev1.Info{Status: releasecommon.StatusDeployed},
					Manifest: crdManifest(tt.release...),
					Config:   copyValues(tt.stored),
				}
			}
			command := tt.command
			if command == "" {
				command = installCommand
				if baseline != nil {
					command = upgradeCommand
				}
			}

			check := gatewayAPICRDCheck{
				releaseName:         contourReleaseName,
				releaseNamespace:    RadiusSystemNamespace,
				kubeContext:         "test-context",
				command:             command,
				getCRDs:             crdLookup(tt.crds),
				renderConfiguration: testRenderConfiguration(tt.capabilities),
			}
			values := copyValues(tt.values)
			warning, err := check.run(t.Context(), helmChart, values, baseline, !tt.resetValues)

			require.Equal(t, tt.wantManageCRDs, nestedValue(values, "gatewayAPI", "manageCRDs"))
			_, valuesDecide := tt.values["gatewayAPI"]
			require.Equal(t, valuesDecide, check.manageCRDsFromValues, "whether the Contour values decide gatewayAPI.manageCRDs")
			require.Equal(t, tt.wantWarning, warning != "", "warning: %q", warning)
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.Equal(t, !tt.wantRenderError, clierrors.IsFriendlyError(err), "the CLI shows the message of a friendly error as is: %v", err)
			for _, want := range tt.wantErr {
				require.Contains(t, err.Error(), want)
			}
			for _, notWant := range tt.wantNotErr {
				require.NotContains(t, err.Error(), notWant)
			}
		})
	}
}

// Test_gatewayAPICRDCheck_SuggestedFlagTurnsOffFeatures verifies that the flag the error suggests parses into the
// Contour arguments that make the check pass, and that it keeps the existing arguments.
func Test_gatewayAPICRDCheck_SuggestedFlagTurnsOffFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		values  map[string]any
		wantKey string
		// wantQuote is the quote character around the suggested value.
		wantQuote string
		wantArgs  []any
	}{
		{
			name:      "no Contour arguments",
			command:   installCommand,
			wantKey:   "contour.extraArgs",
			wantQuote: `"`,
			wantArgs: []any{
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
		{
			name:      "no Contour arguments on upgrade",
			command:   upgradeCommand,
			wantKey:   "contour.extraArgs",
			wantQuote: `"`,
			wantArgs: []any{
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
		{
			name:    "existing Contour arguments are kept",
			command: installCommand,
			values: map[string]any{
				"contour": map[string]any{
					"extraArgs": []any{`--root-namespaces=team-a,team-b`, `--debug={x}\y`, "--disable-feature=tlsroutes"},
				},
			},
			wantKey: "contour.extraArgs",
			// The escaped comma and backslash need single quotes.
			wantQuote: "'",
			wantArgs: []any{
				`--root-namespaces=team-a,team-b`,
				`--debug={x}\y`,
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
		{
			name:    "arguments from a YAML string are kept",
			command: upgradeCommand,
			values: map[string]any{
				"contour": map[string]any{"extraArgs": "- --debug\n- --disable-feature=tcproutes\n"},
			},
			wantKey:   "contour.extraArgs",
			wantQuote: `"`,
			wantArgs: []any{
				"--debug",
				"--disable-feature=tcproutes",
				"--disable-feature=tlsroutes",
				"--disable-feature=backendtlspolicies",
			},
		},
		{
			name:    "contour.args keeps the serve argument",
			command: installCommand,
			values: map[string]any{
				"contour": map[string]any{"args": []any{"serve", "--incluster", "--xds-port=8001"}},
			},
			wantKey:   "contour.args",
			wantQuote: `"`,
			wantArgs: []any{
				"serve",
				"--incluster",
				"--xds-port=8001",
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var baseline *releasev1.Release
			if tt.command == upgradeCommand {
				baseline = &releasev1.Release{Name: contourReleaseName, Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}}
			}
			newCheck := func() *gatewayAPICRDCheck {
				return &gatewayAPICRDCheck{
					releaseName:         contourReleaseName,
					releaseNamespace:    RadiusSystemNamespace,
					command:             tt.command,
					getCRDs:             crdLookup(traefikGatewayAPICRDs()),
					renderConfiguration: testRenderConfiguration(nil),
				}
			}
			values := copyValues(tt.values)
			_, err := newCheck().run(t.Context(), newTestContourChart(), values, baseline, true)
			require.Error(t, err)

			match := regexp.MustCompile(`Add this flag to the (rad [a-z]+ kubernetes) command:\n +--contour-set ("[^"]*"|'[^']*')`).FindStringSubmatch(err.Error())
			require.Len(t, match, 3, "the error must suggest a --contour-set flag: %s", err)
			require.Equal(t, tt.command, match[1])
			require.Equal(t, tt.wantQuote, match[2][:1], "how the suggested value is quoted")
			suggestedValue := unquoteFlagValue(t, match[2])

			suggested := map[string]any{}
			require.NoError(t, strvals.ParseInto(suggestedValue, suggested))
			require.Equal(t, tt.wantArgs, nestedValue(suggested, strings.Split(tt.wantKey, ".")...))

			// Like --contour-set, the suggested value overrides the values of the invocation and makes the check pass.
			values = copyValues(tt.values)
			require.NoError(t, strvals.ParseInto(suggestedValue, values))
			_, err = newCheck().run(t.Context(), newTestContourChart(), values, baseline, true)
			require.NoError(t, err)
		})
	}
}

// Test_gatewayAPICRDCheck_RemedyForContourValuesFromFlags follows the remedy that the check prints when this invocation
// sets the Contour arguments with --contour-set-file or --contour-set. Radius applies every --contour-set flag before
// every --contour-set-file flag, as Helm does, so the remedy must change the flag that sets the arguments.
func Test_gatewayAPICRDCheck_RemedyForContourValuesFromFlags(t *testing.T) {
	t.Parallel()

	for _, command := range []string{installCommand, upgradeCommand} {
		t.Run(command, func(t *testing.T) {
			t.Parallel()

			// check runs the check of command for a k3s cluster with Traefik, with the Contour values that the flags build
			// as buildContourValues builds them. It returns the values that Helm would render the chart with.
			check := func(t *testing.T, setArgs []string, setFileArgs []string) (map[string]any, error) {
				t.Helper()

				values, err := buildContourValues(ContourChartOptions{ChartOptions: ChartOptions{SetArgs: setArgs, SetFileArgs: setFileArgs}})
				require.NoError(t, err)
				var baseline *releasev1.Release
				var reused map[string]any
				if command == upgradeCommand {
					reused = map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}}
					baseline = &releasev1.Release{Name: contourReleaseName, Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Config: reused}
				}
				helmChart := newTestContourChart()
				_, err = (&gatewayAPICRDCheck{
					releaseName:         contourReleaseName,
					releaseNamespace:    RadiusSystemNamespace,
					command:             command,
					setArgs:             setArgs,
					setFileArgs:         setFileArgs,
					getCRDs:             crdLookup(traefikGatewayAPICRDs()),
					renderConfiguration: testRenderConfiguration(nil),
				}).run(t.Context(), helmChart, values, baseline, true)
				if err != nil {
					require.True(t, clierrors.IsFriendlyError(err), "%v", err)
					return nil, err
				}

				effective, err := effectiveContourValues(helmChart, values, reused)
				require.NoError(t, err)
				return effective, nil
			}
			// requireFeaturesTurnedOff verifies that the values turn off the features that the Traefik CRDs lack, and
			// keep the arguments args.
			requireFeaturesTurnedOff := func(t *testing.T, values map[string]any, args ...string) {
				t.Helper()

				contourArgs := contourArgsFromValues(values)
				require.Subset(t, contourArgs.disabledFeatures(), []string{"tlsroutes", "tcproutes", "backendtlspolicies"})
				for _, arg := range args {
					require.Contains(t, contourArgs.args, arg)
				}
			}
			// printedFlag returns the value of the --contour-set flag that err prints on a line of its own.
			printedFlag := func(t *testing.T, err error) string {
				t.Helper()

				match := regexp.MustCompile(`(?m)^ +--contour-set ("[^"\n]*")$`).FindAllStringSubmatch(err.Error(), -1)
				require.Len(t, match, 1, "the error must print one --contour-set flag: %v", err)
				return unquoteFlagValue(t, match[0][1])
			}
			// replacedFileFlags returns the values of the --contour-set-file flags that the error says to replace.
			replacedFileFlags := func(t *testing.T, err error) []string {
				t.Helper()

				phrase := regexp.MustCompile(`[Rr]eplace (--contour-set-file .*?) in the rad [a-z]+ kubernetes command with this flag`).FindStringSubmatch(err.Error())
				require.Len(t, phrase, 2, "the error must say which --contour-set-file flags to replace: %v", err)
				var values []string
				for _, match := range regexp.MustCompile(`--contour-set-file ("[^"]*"|'[^']*')`).FindAllStringSubmatch(phrase[1], -1) {
					values = append(values, unquoteFlagValue(t, match[1]))
				}
				return values
			}
			// without returns flags without the ones in removed.
			without := func(flags []string, removed []string) []string {
				return slices.DeleteFunc(slices.Clone(flags), func(flag string) bool { return slices.Contains(removed, flag) })
			}
			// disabledFeatureEntries returns the YAML list entries that the error says to add to a file.
			disabledFeatureEntries := func(t *testing.T, err error) string {
				t.Helper()

				entries := regexp.MustCompile(`(?m)^ +(- --disable-feature=[a-z]+)$`).FindAllStringSubmatch(err.Error(), -1)
				require.Len(t, entries, 3, "the error must list the entries to add: %v", err)
				var content string
				for _, entry := range entries {
					content += entry[1] + "\n"
				}
				return content
			}

			t.Run("arguments from --contour-set-file", func(t *testing.T) {
				t.Parallel()

				file := filepath.Join(t.TempDir(), "args.yaml")
				require.NoError(t, os.WriteFile(file, []byte("- --debug\n"), 0600))
				setFile := "contour.extraArgs=" + file
				_, err := check(t, nil, []string{setFile})
				require.Error(t, err)
				readFile := strings.TrimPrefix(setFileArg(setFile), "contour.extraArgs=")
				require.Contains(t, err.Error(), "The contour.extraArgs value comes from --contour-set-file "+QuoteFlagValue(setFile)+", and Radius applies --contour-set-file values after --contour-set values")
				require.Contains(t, err.Error(), "Add these entries to the list in "+readFile+":\n"+
					"      - --disable-feature=tlsroutes\n      - --disable-feature=tcproutes\n      - --disable-feature=backendtlspolicies\n")
				require.Contains(t, err.Error(), "Or replace --contour-set-file "+QuoteFlagValue(setFile)+" in the "+command+" command with this flag")
				require.NotContains(t, err.Error(), "Add this flag")
				flag := printedFlag(t, err)
				entries := regexp.MustCompile(`(?m)^ +(- --disable-feature=[a-z]+)$`).FindAllStringSubmatch(err.Error(), -1)
				require.Len(t, entries, 3)

				// Adding the printed flag and keeping --contour-set-file does not work: the file overrides the flag.
				_, err = check(t, []string{flag}, []string{setFile})
				require.ErrorContains(t, err, "Contour 1.32.0 cannot use the Gateway API CRDs in this cluster")

				// Replacing --contour-set-file with the printed flag works.
				values, err := check(t, []string{flag}, nil)
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")

				// Adding the printed entries to the file works too.
				content := "- --debug\n"
				for _, entry := range entries {
					content += entry[1] + "\n"
				}
				require.NoError(t, os.WriteFile(file, []byte(content), 0600))
				values, err = check(t, nil, []string{setFile})
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")
			})

			t.Run("one argument from --contour-set-file", func(t *testing.T) {
				t.Parallel()

				file := filepath.Join(t.TempDir(), "arg.txt")
				require.NoError(t, os.WriteFile(file, []byte("--debug"), 0600))
				setFile := "contour.extraArgs[0]=" + file
				_, err := check(t, nil, []string{setFile})
				require.Error(t, err)
				// The flag sets only the first argument, so a --contour-set flag still decides the list.
				require.Contains(t, err.Error(), "Add this flag to the "+command+" command:")
				require.NotContains(t, err.Error(), "--contour-set-file")
				flag := printedFlag(t, err)
				printed := map[string]any{}
				require.NoError(t, strvals.ParseInto(flag, printed))
				printedArgs, _ := argumentList(nestedValue(printed, "contour", "extraArgs"))
				require.Equal(t, "--debug", printedArgs[0], "the printed list keeps the argument from the file")

				// The flag still sets the first argument after the printed flag, to the same value.
				values, err := check(t, []string{flag}, []string{setFile})
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")
				require.Equal(t, "--debug", contourArgsFromValues(values).args[0])
			})

			t.Run("arguments from a --contour-set-file list of files", func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
				require.NoError(t, os.WriteFile(first, []byte("--debug"), 0600))
				require.NoError(t, os.WriteFile(second, []byte("--incluster"), 0600))
				setFile := "contour.extraArgs={" + first + "," + second + "}"
				_, err := check(t, nil, []string{setFile})
				require.Error(t, err)
				require.Contains(t, err.Error(), "Replace --contour-set-file "+QuoteFlagValue(setFile)+" in the "+command+" command with this flag, which keeps the current arguments:")
				require.NotContains(t, err.Error(), "Add these entries")
				require.NotContains(t, err.Error(), "Add this flag")
				flag := printedFlag(t, err)
				replaced := replacedFileFlags(t, err)
				require.Equal(t, []string{setFile}, replaced)

				// Adding the printed flag and keeping the list of files does not work: the files override the flag.
				_, err = check(t, []string{flag}, []string{setFile})
				require.ErrorContains(t, err, "Contour 1.32.0 cannot use the Gateway API CRDs in this cluster")

				values, err := check(t, []string{flag}, without([]string{setFile}, replaced))
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug", "--incluster")
			})

			t.Run("arguments from two --contour-set-file flags", func(t *testing.T) {
				t.Parallel()

				dir := t.TempDir()
				base, env := filepath.Join(dir, "base.yaml"), filepath.Join(dir, "env.yaml")
				require.NoError(t, os.WriteFile(base, []byte("- --incluster\n"), 0600))
				require.NoError(t, os.WriteFile(env, []byte("- --debug\n"), 0600))
				baseFlag, envFlag := "contour.extraArgs="+base, "contour.extraArgs="+env
				fileFlags := []string{baseFlag, envFlag}
				_, err := check(t, nil, fileFlags)
				require.Error(t, err)
				require.Contains(t, err.Error(), "The contour.extraArgs value comes from --contour-set-file "+QuoteFlagValue(envFlag)+", and Radius applies --contour-set-file values after --contour-set values")
				require.Contains(t, err.Error(), "Add these entries to the list in "+strings.TrimPrefix(setFileArg(envFlag), "contour.extraArgs=")+":")
				require.Contains(t, err.Error(), "Or replace --contour-set-file "+QuoteFlagValue(baseFlag)+" and --contour-set-file "+QuoteFlagValue(envFlag)+" in the "+command+" command")
				flag := printedFlag(t, err)
				entries := disabledFeatureEntries(t, err)
				replaced := replacedFileFlags(t, err)
				require.ElementsMatch(t, fileFlags, replaced)

				// Replacing only the flag that decides the value does not work: the other file overrides the printed flag.
				_, err = check(t, []string{flag}, []string{baseFlag})
				require.ErrorContains(t, err, "Contour 1.32.0 cannot use the Gateway API CRDs in this cluster")

				values, err := check(t, []string{flag}, without(fileFlags, replaced))
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")

				// Adding the printed entries to the file that decides the value works too.
				require.NoError(t, os.WriteFile(env, []byte("- --debug\n"+entries), 0600))
				values, err = check(t, nil, fileFlags)
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")
			})

			t.Run("arguments from --contour-set", func(t *testing.T) {
				t.Parallel()

				set := "contour.extraArgs={--debug}"
				_, err := check(t, []string{set}, nil)
				require.Error(t, err)
				require.Contains(t, err.Error(), "Replace --contour-set "+QuoteFlagValue(set)+" in the "+command+" command with this flag, which keeps its arguments:")
				require.NotContains(t, err.Error(), "Add this flag")
				flag := printedFlag(t, err)

				// Before the original flag, the printed flag does not work: the original flag overrides it.
				_, err = check(t, []string{flag, set}, nil)
				require.ErrorContains(t, err, "Contour 1.32.0 cannot use the Gateway API CRDs in this cluster")

				values, err := check(t, []string{flag}, nil)
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")
			})

			t.Run("arguments from --contour-set that sets other values too", func(t *testing.T) {
				t.Parallel()

				set := "contour.extraArgs={--debug},contour.replicas=3"
				_, err := check(t, []string{set}, nil)
				require.Error(t, err)
				require.Contains(t, err.Error(), "The contour.extraArgs value comes from --contour-set "+QuoteFlagValue(set)+", which also sets other values. Add this flag to the "+command+" command after that flag")
				flag := printedFlag(t, err)

				values, err := check(t, []string{set, flag}, nil)
				require.NoError(t, err)
				requireFeaturesTurnedOff(t, values, "--debug")
				require.EqualValues(t, 3, nestedValue(values, "contour", "replicas"))
			})
		})
	}
}

func Test_gatewayAPICRDCheck_valueSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setArgs     []string
		setFileArgs []string
		want        contourValueSource
		wantFound   bool
	}{
		{
			name: "no flags",
		},
		{
			name:    "flags for other values",
			setArgs: []string{"contour.replicas=3", "contour.extraArgsFoo=x"},
		},
		{
			name:      "--contour-set",
			setArgs:   []string{"contour.extraArgs={--debug}"},
			want:      contourValueSource{value: "contour.extraArgs={--debug}"},
			wantFound: true,
		},
		{
			name:      "--contour-set of one argument",
			setArgs:   []string{"contour.extraArgs[1]=--debug"},
			want:      contourValueSource{value: "contour.extraArgs[1]=--debug"},
			wantFound: true,
		},
		{
			name:      "the last --contour-set decides",
			setArgs:   []string{"contour.extraArgs={--debug}", "contour.replicas=3", "contour.extraArgs={--incluster},contour.replicas=2"},
			want:      contourValueSource{value: "contour.extraArgs={--incluster},contour.replicas=2", otherValues: true},
			wantFound: true,
		},
		{
			name:        "--contour-set-file",
			setArgs:     []string{"contour.extraArgs={--debug}"},
			setFileArgs: []string{"contour.extraArgs=args.yaml"},
			want:        contourValueSource{fromFile: true, value: "contour.extraArgs=args.yaml", file: "args.yaml", fileFlags: []string{"contour.extraArgs=args.yaml"}},
			wantFound:   true,
		},
		{
			name:        "--contour-set-file of a value that contains the arguments",
			setFileArgs: []string{"contour=contour.yaml"},
			want:        contourValueSource{fromFile: true, value: "contour=contour.yaml", fileFlags: []string{"contour=contour.yaml"}},
			wantFound:   true,
		},
		{
			name:        "--contour-set-file that sets other values too",
			setFileArgs: []string{"contour.extraArgs=args.yaml,envoy.service.annotations=annotations.yaml"},
			want: contourValueSource{
				fromFile:    true,
				value:       "contour.extraArgs=args.yaml,envoy.service.annotations=annotations.yaml",
				file:        "args.yaml",
				fileFlags:   []string{"contour.extraArgs=args.yaml,envoy.service.annotations=annotations.yaml"},
				otherValues: true,
			},
			wantFound: true,
		},
		{
			name:        "--contour-set-file for other values",
			setArgs:     []string{"contour.extraArgs={--debug}"},
			setFileArgs: []string{"envoy.service.annotations=annotations.yaml"},
			want:        contourValueSource{value: "contour.extraArgs={--debug}"},
			wantFound:   true,
		},
		{
			// The flag sets one argument and keeps the others, so it does not decide the list.
			name:        "--contour-set-file of one argument",
			setFileArgs: []string{"contour.extraArgs[0]=arg.txt"},
		},
		{
			name:        "--contour-set-file of one argument of a list from --contour-set",
			setArgs:     []string{"contour.extraArgs={--debug,--incluster}"},
			setFileArgs: []string{"contour.extraArgs[0]=arg.txt"},
			want:        contourValueSource{value: "contour.extraArgs={--debug,--incluster}"},
			wantFound:   true,
		},
		{
			name:        "--contour-set-file of a list of files",
			setFileArgs: []string{"contour.extraArgs={a.txt,b.txt}"},
			want:        contourValueSource{fromFile: true, value: "contour.extraArgs={a.txt,b.txt}", fileFlags: []string{"contour.extraArgs={a.txt,b.txt}"}},
			wantFound:   true,
		},
		{
			name:        "two --contour-set-file flags",
			setFileArgs: []string{"contour.extraArgs=base.yaml", "contour.extraArgs=env.yaml"},
			want: contourValueSource{
				fromFile:  true,
				value:     "contour.extraArgs=env.yaml",
				file:      "env.yaml",
				fileFlags: []string{"contour.extraArgs=base.yaml", "contour.extraArgs=env.yaml"},
			},
			wantFound: true,
		},
		{
			name:        "a list of files after a file",
			setFileArgs: []string{"contour.extraArgs=base.yaml", "contour.extraArgs={a.txt,b.txt}", "contour.extraArgs[0]=c.txt"},
			want: contourValueSource{
				fromFile:  true,
				value:     "contour.extraArgs={a.txt,b.txt}",
				fileFlags: []string{"contour.extraArgs=base.yaml", "contour.extraArgs={a.txt,b.txt}"},
			},
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			source, found := gatewayAPICRDCheck{setArgs: tt.setArgs, setFileArgs: tt.setFileArgs}.valueSource("contour.extraArgs")
			require.Equal(t, tt.wantFound, found)
			require.Equal(t, tt.want, source)
		})
	}
}

// Test_gatewayAPICRDCheck_SkipContourInstall verifies that the Gateway API compatibility and conflict errors of rad
// install kubernetes suggest installing Radius without Contour, and that the errors of rad upgrade kubernetes do not.
func Test_gatewayAPICRDCheck_SkipContourInstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		crds     []crdInfo
		release  []crdInfo
		values   map[string]any
		wantLine string
	}{
		{
			name: "Gateway API CRDs that Contour cannot use",
			crds: traefikGatewayAPICRDs(),
			wantLine: "\n  - Install Radius without Contour with --skip-contour-install, for example when Traefik is your ingress controller. Radius gateways use Contour, so they do not work without it." +
				"\n    If you used rad init, run rad install kubernetes --skip-contour-install first, and then run rad init again.",
		},
		{
			name:   "Gateway API CRDs that belong to something else",
			crds:   traefikGatewayAPICRDs(),
			values: map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantLine: "\nTo continue without Contour, install Radius with --skip-contour-install, for example when Traefik is your ingress controller. Radius gateways use Contour, so they do not work without it." +
				" If you used rad init, run rad install kubernetes --skip-contour-install first, and then run rad init again.",
		},
		{
			name:     "Gateway API CRDs with more than one owner",
			crds:     append(withoutCRDs(contourBundleCRDs(), crdNames(traefikGatewayAPICRDs())...), traefikGatewayAPICRDs()...),
			release:  contourBundleCRDs(),
			wantLine: "\nTo continue without Contour, install Radius with --skip-contour-install, for example when Traefik is your ingress controller.",
		},
	}

	for _, tt := range tests {
		for _, command := range []string{installCommand, upgradeCommand} {
			t.Run(tt.name+"/"+command, func(t *testing.T) {
				t.Parallel()

				var baseline *releasev1.Release
				if tt.release != nil || command == upgradeCommand {
					baseline = &releasev1.Release{Name: contourReleaseName, Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusFailed}, Manifest: crdManifest(tt.release...)}
				}
				_, err := (&gatewayAPICRDCheck{
					releaseName:         contourReleaseName,
					releaseNamespace:    RadiusSystemNamespace,
					command:             command,
					getCRDs:             crdLookup(tt.crds),
					renderConfiguration: testRenderConfiguration(nil),
				}).run(t.Context(), newTestContourChart(), copyValues(tt.values), baseline, true)
				require.Error(t, err)
				require.True(t, clierrors.IsFriendlyError(err), "%v", err)
				if command == installCommand {
					require.Contains(t, err.Error(), tt.wantLine)
				} else {
					require.NotContains(t, err.Error(), "--skip-contour-install")
				}
			})
		}
	}
}

// Test_liveContourCluster_RenderConfiguration verifies that the configuration with which the Contour checks render the
// chart reads the cluster that the Kubernetes context selects, as the real install or upgrade does, and that a dry run
// with it can neither apply resources nor record a release.
func Test_liveContourCluster_RenderConfiguration(t *testing.T) {
	t.Parallel()

	configuration, err := liveContourCluster{}.RenderConfiguration("k3d-radius-demo", RadiusSystemNamespace)
	require.NoError(t, err)

	flags, ok := configuration.RESTClientGetter.(*genericclioptions.ConfigFlags)
	require.True(t, ok, "templates must see the capabilities and lookups of the cluster: %T", configuration.RESTClientGetter)
	require.Equal(t, "k3d-radius-demo", *flags.Context)
	require.Equal(t, RadiusSystemNamespace, *flags.Namespace)
	require.Nil(t, configuration.Capabilities, "Helm must discover the capabilities of the cluster")
	require.IsType(t, &kubefake.PrintingKubeClient{}, configuration.KubeClient, "the dry run must not apply resources")
	require.IsType(t, &driver.Memory{}, configuration.Releases.Driver, "the dry run must not record a release in the cluster")
}

func Test_contourUpgradeBaseline(t *testing.T) {
	t.Parallel()

	revision := func(version int, status releasecommon.Status) *releasev1.Release {
		return &releasev1.Release{Name: contourReleaseName, Version: version, Info: &releasev1.Info{Status: status}}
	}

	tests := []struct {
		name         string
		history      []*releasev1.Release
		kubeContext  string
		wantRevision int
		wantErr      []string
		wantFriendly bool
	}{
		{
			name:         "latest revision deployed",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusSuperseded), revision(2, releasecommon.StatusDeployed)},
			wantRevision: 2,
		},
		{
			name:         "failed upgrade on top of a deployed revision",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusSuperseded), revision(2, releasecommon.StatusDeployed), revision(3, releasecommon.StatusFailed)},
			wantRevision: 2,
		},
		{
			name:         "history out of order",
			history:      []*releasev1.Release{revision(10, releasecommon.StatusFailed), revision(9, releasecommon.StatusDeployed), nil, revision(8, releasecommon.StatusSuperseded)},
			wantRevision: 9,
		},
		{
			name:         "several deployed revisions",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusDeployed), revision(3, releasecommon.StatusFailed)},
			wantRevision: 2,
		},
		{
			name:         "only a failed install",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusFailed)},
			wantRevision: 1,
		},
		{
			name:         "failed revisions without a deployed one",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusFailed), revision(2, releasecommon.StatusFailed)},
			wantRevision: 2,
		},
		{
			name:         "superseded latest revision without a deployed one",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusFailed), revision(2, releasecommon.StatusSuperseded)},
			wantRevision: 2,
		},
		{
			name:         "pending upgrade",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusPendingUpgrade)},
			wantErr:      []string{"another operation is in progress on Contour release \"contour\" in namespace \"radius-system\": revision 2 is pending-upgrade", "list the revisions with helm -n radius-system history contour, roll the release back to the last revision that worked, for example with helm -n radius-system rollback contour 1, and then run the command again."},
			wantFriendly: true,
		},
		{
			name:         "pending install",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusPendingInstall)},
			wantErr:      []string{"revision 1 is pending-install", "delete the record of the pending revision with kubectl -n radius-system delete secret sh.helm.release.v1.contour.v1, which does not change the resources in the cluster, and then run the command again."},
			wantFriendly: true,
		},
		{
			name:         "pending upgrade in the cluster of a context",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusPendingUpgrade)},
			kubeContext:  "k3d-radius-demo",
			wantErr:      []string{`list the revisions with helm --kube-context "k3d-radius-demo" -n radius-system history contour, roll the release back to the last revision that worked, for example with helm --kube-context "k3d-radius-demo" -n radius-system rollback contour 1, and then run the command again.`},
			wantFriendly: true,
		},
		{
			name:         "pending install in the cluster of a context that needs single quotes",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusPendingInstall)},
			kubeContext:  "it's $prod",
			wantErr:      []string{`list the revisions with helm --kube-context 'it'\''s $prod' -n radius-system history contour, delete the record of the pending revision with kubectl --context 'it'\''s $prod' -n radius-system delete secret sh.helm.release.v1.contour.v1, which does not change`},
			wantFriendly: true,
		},
		{
			name:         "pending rollback",
			history:      []*releasev1.Release{revision(1, releasecommon.StatusDeployed), revision(2, releasecommon.StatusPendingRollback)},
			wantErr:      []string{"revision 2 is pending-rollback"},
			wantFriendly: true,
		},
		{
			name:    "uninstalled release with its history kept",
			history: []*releasev1.Release{revision(1, releasecommon.StatusSuperseded), revision(2, releasecommon.StatusUninstalled)},
			wantErr: []string{`"contour" has no deployed releases`},
		},
		{
			name:    "revision without status",
			history: []*releasev1.Release{{Name: contourReleaseName, Version: 1}},
			wantErr: []string{`"contour" has no deployed releases`},
		},
		{
			name:    "no history",
			wantErr: []string{`"contour" has no deployed releases`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			baseline, err := contourUpgradeBaseline(tt.history, contourReleaseName, RadiusSystemNamespace, tt.kubeContext)

			// Helm cannot store a revision without a status, so only Radius handles that history.
			if !slices.ContainsFunc(tt.history, func(revision *releasev1.Release) bool { return revision != nil && revision.Info == nil }) {
				helmRevision, helmErr := helmUpgradeBaseline(t, tt.history)
				if err == nil {
					require.NoError(t, helmErr)
					require.Equal(t, helmRevision, baseline.Version, "Helm's upgrade starts from another revision")
				} else {
					require.Error(t, helmErr, "Helm's upgrade starts from revision %d", helmRevision)
				}
			}

			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				require.Equal(t, tt.wantRevision, baseline.Version)
				return
			}

			require.Error(t, err)
			require.Equal(t, tt.wantFriendly, clierrors.IsFriendlyError(err))
			if !tt.wantFriendly {
				require.True(t, errors.Is(err, driver.ErrNoDeployedReleases))
			}
			for _, want := range tt.wantErr {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

// helmUpgradeBaseline returns the revision that Helm's upgrade action starts from for a release with history. It runs
// a client-only upgrade that reuses the values of that revision, which record its number.
func helmUpgradeBaseline(t *testing.T, history []*releasev1.Release) (int, error) {
	t.Helper()

	memory := driver.NewMemory()
	memory.SetNamespace(RadiusSystemNamespace)
	configuration := &helm.Configuration{
		Releases:     storage.Init(memory),
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities: common.DefaultCapabilities.Copy(),
	}
	for _, revision := range history {
		if revision == nil {
			continue
		}
		stored := *revision
		stored.Namespace = RadiusSystemNamespace
		stored.Chart = newTestContourChart()
		stored.Config = map[string]any{"revision": revision.Version}
		require.NoError(t, configuration.Releases.Create(&stored))
	}

	upgrade := helm.NewUpgrade(configuration)
	upgrade.Namespace = RadiusSystemNamespace
	upgrade.DryRunStrategy = helm.DryRunClient
	upgrade.ResetThenReuseValues = true
	upgraded, err := upgrade.RunWithContext(t.Context(), contourReleaseName, newTestContourChart(), map[string]any{})
	if err != nil {
		return 0, err
	}

	release, ok := upgraded.(*releasev1.Release)
	require.True(t, ok)
	revision, ok := release.Config["revision"].(int)
	require.True(t, ok, "the upgrade did not reuse the values of a revision: %v", release.Config)

	return revision, nil
}

// Test_gatewayAPICRDCheck_ReadsOnlyInvolvedCRDs verifies that the check reads only the CRDs that the change involves,
// because a cluster can have many large CRDs.
func Test_gatewayAPICRDCheck_ReadsOnlyInvolvedCRDs(t *testing.T) {
	t.Parallel()

	listenerSets := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "listenersets", "v1")
	clusterCRDs := append(contourBundleCRDs(), listenerSets, crdInfo{name: "widgets.example.com", group: "example.com"})

	tests := []struct {
		name     string
		baseline *releasev1.Release
		wantRead []string
	}{
		{
			name:     "install",
			wantRead: crdNames(contourBundleCRDs()),
		},
		{
			name:     "upgrade of a release that installed another CRD",
			baseline: &releasev1.Release{Name: contourReleaseName, Version: 1, Info: &releasev1.Info{Status: releasecommon.StatusDeployed}, Manifest: crdManifest(listenerSets)},
			wantRead: append(crdNames(contourBundleCRDs()), listenerSets.name),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var requests [][]string
			check := gatewayAPICRDCheck{
				releaseName:      contourReleaseName,
				releaseNamespace: RadiusSystemNamespace,
				command:          installCommand,
				getCRDs: func(ctx context.Context, names []string) ([]crdInfo, error) {
					requests = append(requests, names)
					return crdLookup(clusterCRDs)(ctx, names)
				},
				renderConfiguration: testRenderConfiguration(nil),
			}
			_, _ = check.run(t.Context(), newTestContourChart(), map[string]any{}, tt.baseline, true)

			// The CRDs that the chart renders with the final values are the ones that it renders with
			// gatewayAPI.manageCRDs=true, which the first request already read.
			require.Len(t, requests, 1)
			require.ElementsMatch(t, tt.wantRead, requests[0])
		})
	}
}
func Test_renderChartCRDs(t *testing.T) {
	t.Parallel()

	t.Run("CRDs that the values enable", func(t *testing.T) {
		t.Parallel()

		helmChart := newTestContourChart()
		for _, manageCRDs := range []bool{true, false} {
			rendered, directory, err := renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), helmChart, contourReleaseName, RadiusSystemNamespace, false,
				map[string]any{"gatewayAPI": map[string]any{"manageCRDs": manageCRDs}})
			require.NoError(t, err)
			require.Empty(t, directory)
			if !manageCRDs {
				require.Empty(t, rendered)
				continue
			}

			var want []crdInfo
			for _, crd := range unmanagedCRDs(contourBundleCRDs()) {
				want = append(want, crdInfo{name: crd.name, group: crd.group, versions: crd.servedVersions, servedVersions: crd.servedVersions})
			}
			require.Equal(t, want, rendered)
		}
	})

	t.Run("subcharts and crds/ directories", func(t *testing.T) {
		t.Parallel()

		subchart := &chart.Chart{
			Metadata: &chart.Metadata{APIVersion: "v2", Name: "gateway", Version: "0.1.0"},
			Templates: []*common.File{
				{Name: "templates/crds.yaml", Data: []byte(crdManifest(newUnmanagedCRD("grpcroutes", "v1")))},
			},
			Files: []*common.File{
				{Name: "crds/tlsroutes.yaml", Data: []byte(crdManifest(newUnmanagedCRD("tlsroutes", "v1alpha2")))},
			},
		}
		parent := &chart.Chart{
			Metadata: &chart.Metadata{
				APIVersion:   "v2",
				Name:         "contour",
				Version:      "0.1.0",
				Dependencies: []*chart.Dependency{{Name: "gateway", Version: "0.1.0", Condition: "gateway.enabled"}},
			},
			Values: map[string]any{"gateway": map[string]any{"enabled": true}},
			Templates: []*common.File{
				{Name: "templates/crds.yaml", Data: []byte(crdManifest(newUnmanagedCRD("gateways", "v1")))},
			},
			Files: []*common.File{
				{Name: "crds/tcproutes.yaml", Data: []byte(crdManifest(newUnmanagedCRD("tcproutes", "v1alpha2")))},
			},
		}
		parent.AddDependency(subchart)

		rendered, directory, err := renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), parent, contourReleaseName, RadiusSystemNamespace, false, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"gateways." + gatewayAPIGroup, "grpcroutes." + gatewayAPIGroup}, crdNames(rendered))
		require.ElementsMatch(t, []string{"tcproutes." + gatewayAPIGroup, "tlsroutes." + gatewayAPIGroup}, crdNames(directory))

		// Helm removes a disabled subchart from the chart that it renders, but not from the original chart.
		rendered, directory, err = renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), parent, contourReleaseName, RadiusSystemNamespace, false,
			map[string]any{"gateway": map[string]any{"enabled": false}})
		require.NoError(t, err)
		require.Equal(t, []string{"gateways." + gatewayAPIGroup}, crdNames(rendered))
		require.Equal(t, []string{"tcproutes." + gatewayAPIGroup}, crdNames(directory))

		require.Len(t, parent.Dependencies(), 1)
		require.Same(t, subchart, parent.Dependencies()[0])
		require.Same(t, parent, subchart.Parent())
		require.Len(t, parent.Metadata.Dependencies, 1)
		require.Equal(t, chart.Dependency{Name: "gateway", Version: "0.1.0", Condition: "gateway.enabled"}, *parent.Metadata.Dependencies[0])
		require.Equal(t, map[string]any{"gateway": map[string]any{"enabled": true}}, parent.Values)
	})

	t.Run("template error", func(t *testing.T) {
		t.Parallel()

		broken := newTestContourChart()
		broken.Templates = append(broken.Templates, &common.File{Name: "templates/broken.yaml", Data: []byte(`{{ fail "the values are not supported" }}`)})
		_, _, err := renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), broken, contourReleaseName, RadiusSystemNamespace, false, nil)
		require.ErrorContains(t, err, "the values are not supported")
	})

	t.Run("CRDs that the chart renders only with some capabilities of the cluster", func(t *testing.T) {
		t.Parallel()

		helmChart := newCapabilitiesGatedCRDChart()
		rendered, _, err := renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), helmChart, contourReleaseName, RadiusSystemNamespace, true, nil)
		require.NoError(t, err)
		require.Equal(t, []string{"httpproxies.projectcontour.io"}, crdNames(rendered))

		rendered, _, err = renderChartCRDs(t.Context(), newTestRenderConfiguration(capabilitiesWith("gateway.networking.k8s.io/v1")), helmChart, contourReleaseName, RadiusSystemNamespace, true, nil)
		require.NoError(t, err)
		require.Empty(t, rendered)
	})

	t.Run("CRDs that the chart renders only on install or only on upgrade", func(t *testing.T) {
		t.Parallel()

		helmChart := newInstallOnlyCRDChart()
		helmChart.Templates = append(helmChart.Templates, &common.File{
			Name: "templates/upgrade-only.yaml",
			Data: []byte("{{- if .Release.IsUpgrade }}\n" + crdManifest(newUnmanagedCRD("tcproutes", "v1alpha2")) + "{{- end }}\n"),
		})
		for _, upgrade := range []bool{false, true} {
			rendered, _, err := renderChartCRDs(t.Context(), newTestRenderConfiguration(nil), helmChart, contourReleaseName, RadiusSystemNamespace, upgrade, nil)
			require.NoError(t, err)
			want := []string{"listenersets.gateway.networking.k8s.io"}
			if upgrade {
				want = []string{"tcproutes.gateway.networking.k8s.io"}
			}
			require.Equal(t, want, crdNames(rendered), "upgrade=%t", upgrade)
		}
	})
}

// newTestRenderConfiguration returns a Helm configuration that renders charts with capabilities, or with Helm's default
// capabilities when capabilities is nil. Without a RESTClientGetter, it never contacts a cluster, and template lookups
// find nothing.
func newTestRenderConfiguration(capabilities *common.Capabilities) *helm.Configuration {
	if capabilities == nil {
		capabilities = common.DefaultCapabilities.Copy()
	}

	return &helm.Configuration{
		Capabilities: capabilities,
		KubeClient:   &kubefake.PrintingKubeClient{Out: io.Discard},
		Releases:     storage.Init(driver.NewMemory()),
	}
}

// testRenderConfiguration returns the render configuration of a gatewayAPICRDCheck that renders with capabilities.
func testRenderConfiguration(capabilities *common.Capabilities) func() (*helm.Configuration, error) {
	return func() (*helm.Configuration, error) {
		return newTestRenderConfiguration(capabilities), nil
	}
}

// capabilitiesWith returns Helm's default capabilities with apiVersions added.
func capabilitiesWith(apiVersions ...string) *common.Capabilities {
	capabilities := common.DefaultCapabilities.Copy()
	capabilities.APIVersions = append(capabilities.APIVersions, apiVersions...)
	return capabilities
}

// newCapabilitiesGatedCRDChart returns a Contour chart that renders the HTTPProxy CRD only when the cluster does not
// serve Gateway API v1, and no Gateway API CRDs.
func newCapabilitiesGatedCRDChart() *chart.Chart {
	helmChart := newStubContourChart("1.33.0", nil)
	helmChart.Templates = append(helmChart.Templates, &common.File{
		Name: "templates/httpproxies.yaml",
		Data: []byte(`{{- if not (.Capabilities.APIVersions.Has "gateway.networking.k8s.io/v1") }}` + "\n" +
			crdManifest(crdInfo{name: "httpproxies.projectcontour.io", group: "projectcontour.io", servedVersions: []string{"v1"}}) + "{{- end }}\n"),
	})
	return helmChart
}

// newInstallOnlyCRDChart returns a Contour chart that renders a Gateway API CRD only when Helm installs it, and no
// other Gateway API CRDs.
func newInstallOnlyCRDChart() *chart.Chart {
	helmChart := newStubContourChart("1.32.0", nil)
	helmChart.Templates = append(helmChart.Templates, &common.File{
		Name: "templates/install-only.yaml",
		Data: []byte("{{- if not .Release.IsUpgrade }}\n" + crdManifest(newUnmanagedCRD("listenersets", "v1")) + "{{- end }}\n"),
	})
	return helmChart
}

func Test_contourArgsFromValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		values         map[string]any
		wantKey        string
		wantArgs       []any
		wantFeatures   []string
		wantUnreadable bool
	}{
		{
			name:    "no arguments",
			wantKey: "contour.extraArgs",
		},
		{
			name: "both forms of --disable-feature",
			values: map[string]any{"contour": map[string]any{"extraArgs": []any{
				"--disable-feature=tlsroutes", "--debug", int64(5), "--disable-feature", "tcproutes", "--disable-feature",
			}}},
			wantKey: "contour.extraArgs",
			wantArgs: []any{
				"--disable-feature=tlsroutes", "--debug", int64(5), "--disable-feature", "tcproutes", "--disable-feature",
			},
			wantFeatures: []string{"tlsroutes", "tcproutes"},
		},
		{
			name: "contour.args replaces the chart's arguments",
			values: map[string]any{"contour": map[string]any{
				"args":      []any{"serve", "--disable-feature=grpcroutes"},
				"extraArgs": []any{"--disable-feature=tlsroutes"},
			}},
			wantKey:      "contour.args",
			wantArgs:     []any{"serve", "--disable-feature=grpcroutes"},
			wantFeatures: []string{"grpcroutes"},
		},
		{
			name: "empty contour.args does not replace the chart's arguments",
			values: map[string]any{"contour": map[string]any{
				"args":      []any{},
				"extraArgs": []any{"--disable-feature=tlsroutes"},
			}},
			wantKey:      "contour.extraArgs",
			wantArgs:     []any{"--disable-feature=tlsroutes"},
			wantFeatures: []string{"tlsroutes"},
		},
		{
			name:         "extraArgs as a YAML block list in a string",
			values:       map[string]any{"contour": map[string]any{"extraArgs": "- --disable-feature=tlsroutes\n- --log-level=7\n- 5\n"}},
			wantKey:      "contour.extraArgs",
			wantArgs:     []any{"--disable-feature=tlsroutes", "--log-level=7", int64(5)},
			wantFeatures: []string{"tlsroutes"},
		},
		{
			name:         "args as a YAML flow list in a string",
			values:       map[string]any{"contour": map[string]any{"args": "[serve, --disable-feature, tcproutes]"}},
			wantKey:      "contour.args",
			wantArgs:     []any{"serve", "--disable-feature", "tcproutes"},
			wantFeatures: []string{"tcproutes"},
		},
		{
			name:    "empty string",
			values:  map[string]any{"contour": map[string]any{"extraArgs": " "}},
			wantKey: "contour.extraArgs",
		},
		{
			name:           "extraArgs that is a single argument instead of a list",
			values:         map[string]any{"contour": map[string]any{"extraArgs": "--disable-feature=tlsroutes"}},
			wantKey:        "contour.extraArgs",
			wantUnreadable: true,
		},
		{
			name:           "args in a string that is not YAML",
			values:         map[string]any{"contour": map[string]any{"args": "[serve, --incluster"}},
			wantKey:        "contour.args",
			wantUnreadable: true,
		},
		{
			name:           "extraArgs that is a table",
			values:         map[string]any{"contour": map[string]any{"extraArgs": map[string]any{"debug": true}}},
			wantKey:        "contour.extraArgs",
			wantUnreadable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := contourArgsFromValues(tt.values)
			require.Equal(t, tt.wantKey, args.key)
			require.Equal(t, tt.wantArgs, args.args)
			require.Equal(t, tt.wantFeatures, args.disabledFeatures())
			require.Equal(t, tt.wantUnreadable, args.unreadable)
		})
	}
}

func Test_effectiveContourValues(t *testing.T) {
	t.Parallel()

	newChart := func() *chart.Chart {
		return &chart.Chart{
			Metadata: &chart.Metadata{Name: "contour", Version: "0.1.0", AppVersion: "1.32.0"},
			Values: map[string]any{
				"contour":    map[string]any{"extraArgs": []any{}, "manageCRDs": true},
				"gatewayAPI": map[string]any{"manageCRDs": false},
			},
		}
	}
	storedValues := func() map[string]any {
		return map[string]any{
			"contour":    map[string]any{"extraArgs": []any{"--disable-feature=tlsroutes"}},
			"gatewayAPI": map[string]any{"manageCRDs": true},
		}
	}

	tests := []struct {
		name           string
		values         map[string]any
		reused         map[string]any
		wantExtraArgs  any
		wantManageCRDs any
	}{
		{
			name:           "install uses the chart defaults beneath the values",
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantExtraArgs:  []any{},
			wantManageCRDs: true,
		},
		{
			name:           "reused values are merged beneath the values",
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			reused:         storedValues(),
			wantExtraArgs:  []any{"--disable-feature=tlsroutes"},
			wantManageCRDs: false,
		},
		{
			name: "values replace a reused list",
			values: map[string]any{
				"contour":    map[string]any{"extraArgs": []any{"--disable-feature=tcproutes"}},
				"gatewayAPI": map[string]any{"manageCRDs": false},
			},
			reused:         storedValues(),
			wantExtraArgs:  []any{"--disable-feature=tcproutes"},
			wantManageCRDs: false,
		},
		{
			name:           "null removes a reused value",
			values:         map[string]any{"contour": map[string]any{"extraArgs": nil}},
			reused:         storedValues(),
			wantExtraArgs:  []any{},
			wantManageCRDs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helmChart := newChart()
			values := copyValues(tt.values)
			reused := copyValues(tt.reused)
			if tt.reused == nil {
				reused = nil
			}

			effective, err := effectiveContourValues(helmChart, values, reused)
			require.NoError(t, err)
			require.Equal(t, tt.wantExtraArgs, nestedValue(effective, "contour", "extraArgs"))
			require.Equal(t, tt.wantManageCRDs, nestedValue(effective, "gatewayAPI", "manageCRDs"))
			require.Equal(t, true, nestedValue(effective, "contour", "manageCRDs"))

			require.Equal(t, tt.values, values, "the values must not change")
			require.Equal(t, tt.reused, reused, "the reused values must not change")
			require.Equal(t, newChart().Values, helmChart.Values, "the chart values must not change")
		})
	}
}

func Test_setDefaultValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		values  map[string]any
		want    map[string]any
		wantSet bool
	}{
		{
			name:    "not set",
			values:  map[string]any{},
			want:    map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
			wantSet: true,
		},
		{
			name:    "parent table without the value",
			values:  map[string]any{"gatewayAPI": map[string]any{"other": "x"}},
			want:    map[string]any{"gatewayAPI": map[string]any{"other": "x", "manageCRDs": true}},
			wantSet: true,
		},
		{
			name:   "set to false",
			values: map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			want:   map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
		},
		{
			name:   "set to null",
			values: map[string]any{"gatewayAPI": map[string]any{"manageCRDs": nil}},
			want:   map[string]any{"gatewayAPI": map[string]any{"manageCRDs": nil}},
		},
		{
			name:   "parent table replaced with null",
			values: map[string]any{"gatewayAPI": nil},
			want:   map[string]any{"gatewayAPI": nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.wantSet, setDefaultValue(tt.values, true, "gatewayAPI", "manageCRDs"))
			require.Equal(t, tt.want, tt.values)
		})
	}
}

func Test_setValue(t *testing.T) {
	t.Parallel()

	for _, values := range []map[string]any{
		{},
		{"gatewayAPI": map[string]any{"manageCRDs": false, "other": "x"}},
		{"gatewayAPI": nil},
		{"gatewayAPI": "not a table"},
	} {
		setValue(values, true, "gatewayAPI", "manageCRDs")
		require.Equal(t, true, nestedValue(values, "gatewayAPI", "manageCRDs"))
	}
}

func Test_crdsInManifest(t *testing.T) {
	t.Parallel()

	manifest := `---
# Source: contour/templates/crds/gateway-api-crds.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gateways.gateway.networking.k8s.io
spec:
  group: gateway.networking.k8s.io
  versions:
  - name: v1
    served: true
  - name: v1beta1
    served: true
  - name: v1alpha2
    served: false
---
# Source: contour/templates/crds/contour-crds.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: httpproxies.projectcontour.io
spec:
  group: projectcontour.io
  versions:
  - name: v1
    served: true
---
# Source: contour/templates/NOTES.txt
---
apiVersion: v1
kind: Service
metadata:
  name: contour
spec:
  ports:
  - port: 8001
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: xlistenersets.gateway.networking.x-k8s.io
spec:
  group: gateway.networking.x-k8s.io
`

	crds, err := crdsInManifest(manifest)
	require.NoError(t, err)
	require.Equal(t, []crdInfo{
		{name: "gateways.gateway.networking.k8s.io", group: gatewayAPIGroup, versions: []string{"v1", "v1beta1", "v1alpha2"}, servedVersions: []string{"v1", "v1beta1"}},
		{name: "httpproxies.projectcontour.io", group: "projectcontour.io", versions: []string{"v1"}, servedVersions: []string{"v1"}},
		{name: "xlistenersets.gateway.networking.x-k8s.io", group: gatewayAPIExperimentalGroup},
	}, crds)
	require.True(t, crds[0].isGatewayAPI())
	require.False(t, crds[1].isGatewayAPI())
	require.True(t, crds[2].isGatewayAPI())

	crds, err = crdsInManifest("")
	require.NoError(t, err)
	require.Empty(t, crds)

	_, err = crdsInManifest("kind: [")
	require.Error(t, err)
}

func Test_getCRDsWithClient(t *testing.T) {
	t.Parallel()

	newCRD := func(name string, group string, versions ...apiextensionsv1.CustomResourceDefinitionVersion) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:   name,
				Labels: map[string]string{helmManagedByLabel: helmManagedByValue, "app.kubernetes.io/name": "traefik"},
				Annotations: map[string]string{
					helmReleaseNameAnnotation:                          "traefik-crd",
					helmReleaseNamespaceAnnotation:                     "kube-system",
					"kubectl.kubernetes.io/last-applied-configuration": "{}",
				},
			},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: group, Versions: versions},
		}
	}
	served := func(name string) apiextensionsv1.CustomResourceDefinitionVersion {
		return apiextensionsv1.CustomResourceDefinitionVersion{Name: name, Served: true}
	}
	notServed := func(name string) apiextensionsv1.CustomResourceDefinitionVersion {
		return apiextensionsv1.CustomResourceDefinitionVersion{Name: name}
	}

	backendTLSPolicies := newCRD("backendtlspolicies.gateway.networking.k8s.io", gatewayAPIGroup, served("v1"), notServed("v1alpha3"))
	backendTLSPolicies.Status.StoredVersions = []string{"v1alpha3", "v1"}
	unmanaged := newCRD("httpproxies.projectcontour.io", "projectcontour.io", served("v1"))
	unmanaged.Labels = nil
	unmanaged.Annotations = map[string]string{kube.ResourcePolicyAnno: kube.KeepPolicy}
	unrelated := newCRD("widgets.example.com", "example.com", served("v1"))

	client := apiextensionsfake.NewClientset(backendTLSPolicies, unmanaged, unrelated)
	var requested []string
	client.PrependReactor("get", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		requested = append(requested, action.(k8stesting.GetAction).GetName())
		return false, nil, nil
	})
	client.PrependReactor("list", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		t.Error("the CRDs must be read by name, because a cluster can have many large CRDs")
		return true, nil, errors.New("unexpected list")
	})

	names := []string{"backendtlspolicies.gateway.networking.k8s.io", "tlsroutes.gateway.networking.k8s.io", "httpproxies.projectcontour.io"}
	got, err := getCRDsWithClient(t.Context(), client, names)
	require.NoError(t, err)
	require.Equal(t, names, requested)

	// CRDs that do not exist are skipped, and only the Helm metadata is kept.
	backendTLSPoliciesInfo := newHelmOwnedCRD("traefik-crd", "kube-system", "backendtlspolicies", "v1")
	backendTLSPoliciesInfo.versions = []string{"v1", "v1alpha3"}
	backendTLSPoliciesInfo.storedVersions = []string{"v1alpha3", "v1"}
	require.Equal(t, []crdInfo{
		backendTLSPoliciesInfo,
		{
			name:           "httpproxies.projectcontour.io",
			group:          "projectcontour.io",
			versions:       []string{"v1"},
			servedVersions: []string{"v1"},
			annotations:    map[string]string{kube.ResourcePolicyAnno: kube.KeepPolicy},
		},
	}, got)
	require.True(t, got[1].kept())
	require.False(t, got[0].kept())
}

func Test_getCRDsWithClient_Error(t *testing.T) {
	t.Parallel()

	client := apiextensionsfake.NewClientset()
	client.PrependReactor("get", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	_, err := getCRDsWithClient(t.Context(), client, []string{"gateways.gateway.networking.k8s.io"})
	require.ErrorContains(t, err, "forbidden")
}
func Test_crdInfo_owner(t *testing.T) {
	t.Parallel()

	require.Equal(t, `Helm release "traefik-crd" in namespace "kube-system"`, newHelmOwnedCRD("traefik-crd", "kube-system", "gateways").owner())

	argo := newUnmanagedCRD("gateways")
	argo.labels = map[string]string{helmManagedByLabel: "argocd"}
	require.Equal(t, `"argocd"`, argo.owner())
	require.False(t, argo.ownedBy(contourReleaseName, RadiusSystemNamespace))

	require.Equal(t, "an installer other than Helm", newUnmanagedCRD("gateways").owner())

	// Helm also requires the managed-by label before it adopts a resource.
	unlabeled := newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "gateways")
	unlabeled.labels = nil
	require.False(t, unlabeled.ownedBy(contourReleaseName, RadiusSystemNamespace))
	require.True(t, newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, "gateways").ownedBy(contourReleaseName, RadiusSystemNamespace))
	require.False(t, newHelmOwnedCRD(contourReleaseName, "projectcontour", "gateways").ownedBy(contourReleaseName, RadiusSystemNamespace))
}

func Test_joinWords(t *testing.T) {
	t.Parallel()

	require.Equal(t, "", joinWords(nil))
	require.Equal(t, "A", joinWords([]string{"A"}))
	require.Equal(t, "A and B", joinWords([]string{"A", "B"}))
	require.Equal(t, "A, B, and C", joinWords([]string{"A", "B", "C"}))
}

func newGatewayAPICRD(resource string, labels map[string]string, annotations map[string]string, servedVersions ...string) crdInfo {
	return crdInfo{
		name:           resource + "." + gatewayAPIGroup,
		group:          gatewayAPIGroup,
		labels:         labels,
		annotations:    annotations,
		servedVersions: servedVersions,
	}
}

func newHelmOwnedCRD(releaseName string, releaseNamespace string, resource string, servedVersions ...string) crdInfo {
	return newGatewayAPICRD(resource,
		map[string]string{helmManagedByLabel: helmManagedByValue},
		map[string]string{helmReleaseNameAnnotation: releaseName, helmReleaseNamespaceAnnotation: releaseNamespace},
		servedVersions...)
}

func newUnmanagedCRD(resource string, servedVersions ...string) crdInfo {
	return newGatewayAPICRD(resource, nil, nil, servedVersions...)
}

// traefikGatewayAPICRDs returns the standard channel Gateway API v1.4.0 CRDs that k3s v1.35 installs with its
// traefik-crd Helm release. They do not include TLSRoute or TCPRoute, and BackendTLSPolicy serves only v1.
func traefikGatewayAPICRDs() []crdInfo {
	crd := func(resource string, servedVersions ...string) crdInfo {
		return newHelmOwnedCRD("traefik-crd", "kube-system", resource, servedVersions...)
	}
	return []crdInfo{
		crd("backendtlspolicies", "v1"),
		crd("gatewayclasses", "v1", "v1beta1"),
		crd("gateways", "v1", "v1beta1"),
		crd("grpcroutes", "v1"),
		crd("httproutes", "v1", "v1beta1"),
		crd("referencegrants", "v1beta1"),
	}
}

// contourBundleCRDs returns the experimental channel Gateway API v1.2.1 CRDs that the Contour 1.32.0 chart installs,
// owned by the Radius Contour release.
func contourBundleCRDs() []crdInfo {
	crd := func(resource string, servedVersions ...string) crdInfo {
		return newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, resource, servedVersions...)
	}
	return []crdInfo{
		crd("backendlbpolicies", "v1alpha2"),
		crd("backendtlspolicies", "v1alpha3"),
		crd("gatewayclasses", "v1", "v1beta1"),
		crd("gateways", "v1", "v1beta1"),
		crd("grpcroutes", "v1"),
		crd("httproutes", "v1", "v1beta1"),
		crd("referencegrants", "v1beta1"),
		crd("tcproutes", "v1alpha2"),
		crd("tlsroutes", "v1alpha2"),
		crd("udproutes", "v1alpha2"),
	}
}

// withoutCRDs returns crds without the CRDs with the given names.
func withoutCRDs(crds []crdInfo, names ...string) []crdInfo {
	return slices.DeleteFunc(slices.Clone(crds), func(crd crdInfo) bool { return slices.Contains(names, crd.name) })
}

// keptCRDs returns crds with the helm.sh/resource-policy: keep annotation.
func keptCRDs(crds []crdInfo) []crdInfo {
	kept := make([]crdInfo, 0, len(crds))
	for _, crd := range crds {
		annotations := map[string]string{kube.ResourcePolicyAnno: kube.KeepPolicy}
		for key, value := range crd.annotations {
			annotations[key] = value
		}
		crd.annotations = annotations
		kept = append(kept, crd)
	}
	return kept
}

// unmanagedCRDs returns crds without ownership metadata, as kubectl apply installs them.
func unmanagedCRDs(crds []crdInfo) []crdInfo {
	for i := range crds {
		crds[i].labels = nil
		crds[i].annotations = nil
	}
	return crds
}

// crdLookup returns a CRD reader for gatewayAPICRDCheck that reads crds without contacting a cluster.
func crdLookup(crds []crdInfo) func(context.Context, []string) ([]crdInfo, error) {
	return func(_ context.Context, names []string) ([]crdInfo, error) {
		var found []crdInfo
		for _, name := range names {
			if crd, ok := findCRD(crds, name); ok {
				found = append(found, crd)
			}
		}
		return found, nil
	}
}

// replaceCRD returns crds with the CRD that has the name of replacement replaced by it.
func replaceCRD(crds []crdInfo, replacement crdInfo) []crdInfo {
	replaced := slices.Clone(crds)
	for i := range replaced {
		if replaced[i].name == replacement.name {
			replaced[i] = replacement
		}
	}
	return replaced
}

// storedIn returns crd with the given versions, the first of which the API server serves, and with objects stored in
// storedVersions.
func storedIn(crd crdInfo, versions []string, storedVersions ...string) crdInfo {
	crd.versions = versions
	crd.servedVersions = versions[:1]
	crd.storedVersions = storedVersions
	return crd
}

// newTestContourChart returns a chart like the default Contour chart, which deploys Contour 1.32.0 and renders its
// Gateway API CRDs only when gatewayAPI.manageCRDs is true.
func newTestContourChart() *chart.Chart {
	return newStubContourChart("1.32.0", contourBundleCRDs())
}

// newStubContourChart returns a Contour chart that deploys the given Contour version and, like the Contour chart,
// renders gatewayCRDs only when gatewayAPI.manageCRDs is true.
func newStubContourChart(appVersion string, gatewayCRDs []crdInfo) *chart.Chart {
	return &chart.Chart{
		Metadata: &chart.Metadata{APIVersion: "v2", Name: "contour", Version: ContourChartDefaultVersion, AppVersion: appVersion},
		Values: map[string]any{
			"contour":    map[string]any{"extraArgs": []any{}},
			"gatewayAPI": map[string]any{"manageCRDs": false},
		},
		Templates: []*common.File{
			{Name: "templates/crds/gateway-api-crds.yaml", Data: []byte(gatedCRDTemplate(gatewayCRDs))},
		},
	}
}

// gatedCRDTemplate returns a chart template that renders crds only when gatewayAPI.manageCRDs is true, like the
// Gateway API CRD template of the Contour chart.
func gatedCRDTemplate(crds []crdInfo) string {
	unowned := make([]crdInfo, 0, len(crds))
	for _, crd := range crds {
		crd.labels = nil
		crd.annotations = nil
		unowned = append(unowned, crd)
	}

	return "{{- if .Values.gatewayAPI.manageCRDs }}\n" + crdManifest(unowned...) + "{{- end }}\n"
}

// crdManifest returns a manifest with a CRD for each of crds.
func crdManifest(crds ...crdInfo) string {
	var manifest strings.Builder
	for _, crd := range crds {
		document, err := yaml.Marshal(newTestCRD(crd))
		if err != nil {
			panic(err)
		}
		manifest.WriteString("---\n")
		manifest.Write(document)
	}

	return manifest.String()
}

// testCRDKinds are the kinds of the CRDs that the tests create, by plural name.
var testCRDKinds = map[string]string{
	"backendlbpolicies":  "BackendLBPolicy",
	"backendtlspolicies": "BackendTLSPolicy",
	"gatewayclasses":     "GatewayClass",
	"gateways":           "Gateway",
	"grpcroutes":         "GRPCRoute",
	"helmcharts":         "HelmChart",
	"httpproxies":        "HTTPProxy",
	"httproutes":         "HTTPRoute",
	"ingressroutes":      "IngressRoute",
	"listenersets":       "ListenerSet",
	"referencegrants":    "ReferenceGrant",
	"tcproutes":          "TCPRoute",
	"tlsroutes":          "TLSRoute",
	"udproutes":          "UDPRoute",
	"xlistenersets":      "XListenerSet",
}

// newTestCRD returns a CRD with the name, group, Helm metadata, and served versions of crd.
func newTestCRD(crd crdInfo) *apiextensionsv1.CustomResourceDefinition {
	plural, group, _ := strings.Cut(crd.name, ".")
	kind, found := testCRDKinds[plural]
	if !found || group != crd.group {
		panic(fmt.Sprintf("add the kind of %s to testCRDKinds, and set its group", crd.name))
	}

	scope := apiextensionsv1.NamespaceScoped
	if kind == "GatewayClass" {
		scope = apiextensionsv1.ClusterScoped
	}
	annotations := map[string]string{}
	// The API server requires this annotation for CRDs in protected groups. The Gateway API CRDs in such groups have it.
	if strings.HasSuffix(group, ".k8s.io") || strings.HasSuffix(group, ".kubernetes.io") {
		annotations["api-approved.kubernetes.io"] = "https://github.com/kubernetes-sigs/gateway-api/pull/3328"
	}
	for key, value := range crd.annotations {
		annotations[key] = value
	}

	preserveUnknownFields := true
	object := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        crd.name,
			Labels:      crd.labels,
			Annotations: annotations,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: strings.ToLower(kind),
				Kind:     kind,
				ListKind: kind + "List",
			},
			Scope: scope,
		},
	}
	versions := crd.versions
	if versions == nil {
		versions = crd.servedVersions
	}
	for i, version := range versions {
		object.Spec.Versions = append(object.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name:    version,
			Served:  slices.Contains(crd.servedVersions, version),
			Storage: i == 0,
			Schema: &apiextensionsv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &preserveUnknownFields},
			},
		})
	}

	return object
}
