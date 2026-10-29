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
		name            string
		chart           *chart.Chart
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
			wantNotErr: []string{"--contour-set", "rad init"},
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
			name: "CRDs with more than one owner",
			crds: append(contourBundleCRDs()[:2],
				newHelmOwnedCRD("traefik-crd", "kube-system", "gateways", "v1")),
			wantManageCRDs: true,
			wantErr: []string{
				"have more than one owner",
				`Contour release "contour" in namespace "radius-system" manages these CRDs:`,
				`gateways.gateway.networking.k8s.io (owned by Helm release "traefik-crd" in namespace "kube-system")`,
			},
		},
		{
			name:           "CRDs installed by the Contour release and then taken over by another release",
			crds:           traefikGatewayAPICRDs(),
			release:        contourBundleCRDs(),
			wantManageCRDs: true,
			wantErr:        []string{"have more than one owner"},
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
				"kubectl annotate crd backendlbpolicies.gateway.networking.k8s.io backendtlspolicies.gateway.networking.k8s.io",
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
				"kubectl annotate crd listenersets.gateway.networking.k8s.io helm.sh/resource-policy=keep",
			},
			wantNotErr: []string{"gateways.gateway.networking.k8s.io", "Remove the Contour value"},
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
				releaseName:      contourReleaseName,
				releaseNamespace: RadiusSystemNamespace,
				command:          command,
				getCRDs:          crdLookup(tt.crds),
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
					releaseName:      contourReleaseName,
					releaseNamespace: RadiusSystemNamespace,
					command:          tt.command,
					getCRDs:          crdLookup(traefikGatewayAPICRDs()),
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

func Test_contourUpgradeBaseline(t *testing.T) {
	t.Parallel()

	revision := func(version int, status releasecommon.Status) *releasev1.Release {
		return &releasev1.Release{Name: contourReleaseName, Version: version, Info: &releasev1.Info{Status: status}}
	}

	tests := []struct {
		name         string
		history      []*releasev1.Release
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
			wantErr:      []string{"revision 1 is pending-install", "for example with helm -n radius-system rollback contour <revision>,"},
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

			baseline, err := contourUpgradeBaseline(tt.history, contourReleaseName, RadiusSystemNamespace)

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
			rendered, directory, err := renderChartCRDs(t.Context(), helmChart, contourReleaseName, RadiusSystemNamespace, false,
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

		rendered, directory, err := renderChartCRDs(t.Context(), parent, contourReleaseName, RadiusSystemNamespace, false, nil)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"gateways." + gatewayAPIGroup, "grpcroutes." + gatewayAPIGroup}, crdNames(rendered))
		require.ElementsMatch(t, []string{"tcproutes." + gatewayAPIGroup, "tlsroutes." + gatewayAPIGroup}, crdNames(directory))

		// Helm removes a disabled subchart from the chart that it renders, but not from the original chart.
		rendered, directory, err = renderChartCRDs(t.Context(), parent, contourReleaseName, RadiusSystemNamespace, false,
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
		_, _, err := renderChartCRDs(t.Context(), broken, contourReleaseName, RadiusSystemNamespace, false, nil)
		require.ErrorContains(t, err, "the values are not supported")
	})
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
	"httpproxies":        "HTTPProxy",
	"httproutes":         "HTTPRoute",
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
