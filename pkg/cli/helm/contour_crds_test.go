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
	"regexp"
	"strconv"
	"testing"

	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/strvals"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// contourChartAppVersions maps each Contour chart version that Radius installs by default to the Contour version that
// the chart deploys, which is its appVersion.
var contourChartAppVersions = map[string]string{
	"0.1.0": "1.32.0",
}

// Test_ContourChartDefaultVersion_HasGatewayAPISupport fails when the default Contour chart changes without a review of
// the Gateway API CRDs that the new Contour version installs and watches.
func Test_ContourChartDefaultVersion_HasGatewayAPISupport(t *testing.T) {
	t.Parallel()

	appVersion, found := contourChartAppVersions[ContourChartDefaultVersion]
	require.Truef(t, found, "add Contour chart %s and its appVersion to contourChartAppVersions", ContourChartDefaultVersion)

	support, found := contourGatewayAPISupportByVersion[appVersion]
	require.Truef(t, found, "add the Gateway API CRDs that Contour %s installs and watches to contourGatewayAPISupportByVersion", appVersion)

	for _, requirement := range support.requirements {
		require.Contains(t, support.bundledCRDs, requirement.crd, "the chart must install every CRD that Contour watches")
	}
}

func Test_gatewayAPICRDCheck(t *testing.T) {
	t.Parallel()

	turnOffUnsupportedFeatures := map[string]any{
		"contour": map[string]any{
			"extraArgs": []any{"--disable-feature=tlsroutes", "--disable-feature=tcproutes", "--disable-feature=backendtlspolicies"},
		},
	}
	traefikWithoutHTTPRoutes := []gatewayAPICRD{}
	for _, crd := range traefikGatewayAPICRDs() {
		if crd.name != "httproutes."+gatewayAPIGroup {
			traefikWithoutHTTPRoutes = append(traefikWithoutHTTPRoutes, crd)
		}
	}

	tests := []struct {
		name           string
		crds           []gatewayAPICRD
		releaseCRDs    []string
		unknownVersion bool
		upgrade        bool
		resetValues    bool
		values         map[string]any
		wantManageCRDs bool
		wantErr        []string
		wantNotErr     []string
		wantWarning    bool
	}{
		{
			name:           "fresh cluster",
			wantManageCRDs: true,
		},
		{
			name:           "CRDs installed by the Contour release",
			crds:           contourBundleCRDs(),
			releaseCRDs:    contourBundleCRDNames(),
			upgrade:        true,
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
				`belong to Helm release "traefik-crd" in namespace "kube-system"`,
				"Contour then ignores TLSRoute, TCPRoute, and BackendTLSPolicy resources.",
				"Add this flag to the rad install kubernetes command:",
				"--contour-set 'contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}'",
				`--k3s-arg "--disable=traefik@server:*"`,
			},
			wantNotErr: []string{"grpcroutes", "gatewayclasses"},
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
			name:           "upgrade with CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			wantManageCRDs: false,
			wantErr: []string{
				"Set this value on the Contour release:",
				"contour.extraArgs={--disable-feature=tlsroutes,--disable-feature=tcproutes,--disable-feature=backendtlspolicies}",
			},
			wantNotErr: []string{"--contour-set", "--reset-values"},
		},
		{
			name:           "upgrade with --reset-values and CRDs from the k3s Traefik chart",
			crds:           traefikGatewayAPICRDs(),
			upgrade:        true,
			resetValues:    true,
			wantManageCRDs: false,
			wantErr:        []string{"run the upgrade without --reset-values"},
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
			releaseCRDs:    contourBundleCRDNames(),
			upgrade:        true,
			wantManageCRDs: true,
			wantErr:        []string{"have more than one owner"},
		},
		{
			name:           "foreign CRD that the chart does not install",
			crds:           []gatewayAPICRD{newUnmanagedCRD("listenersets", "v1")},
			wantManageCRDs: true,
		},
		{
			name:           "CRDs installed by the Contour release and a foreign CRD that the chart does not install",
			crds:           append(contourBundleCRDs(), newUnmanagedCRD("listenersets", "v1")),
			releaseCRDs:    contourBundleCRDNames(),
			upgrade:        true,
			wantManageCRDs: true,
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
			releaseCRDs:    contourBundleCRDNames(),
			upgrade:        true,
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr:        []string{"make Helm delete these CRDs", "gateways.gateway.networking.k8s.io"},
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user with adoptable CRDs that no release installed",
			crds:           contourBundleCRDs(),
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
		},
		{
			name:           "gatewayAPI.manageCRDs=false set by the user on a fresh cluster",
			values:         map[string]any{"gatewayAPI": map[string]any{"manageCRDs": false}},
			wantManageCRDs: false,
			wantErr:        []string{"Contour does not install Gateway API CRDs because gatewayAPI.manageCRDs is false."},
		},
		{
			name:           "unknown Contour version with foreign CRDs",
			crds:           traefikGatewayAPICRDs(),
			unknownVersion: true,
			wantManageCRDs: false,
			wantWarning:    true,
		},
		{
			name:           "unknown Contour version treats every foreign Gateway API CRD as a conflict",
			crds:           []gatewayAPICRD{newUnmanagedCRD("listenersets", "v1")},
			unknownVersion: true,
			wantManageCRDs: false,
			wantWarning:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			check := gatewayAPICRDCheck{
				releaseName:      contourReleaseName,
				releaseNamespace: RadiusSystemNamespace,
				contourVersion:   "1.32.0",
				crds:             tt.crds,
				releaseCRDs:      tt.releaseCRDs,
				upgrade:          tt.upgrade,
				resetValues:      tt.resetValues,
			}
			if tt.unknownVersion {
				check.contourVersion = "9.9.9"
			} else {
				support := contourGatewayAPISupportByVersion[check.contourVersion]
				check.support = &support
			}

			values := copyValues(tt.values)
			setDefaultValue(values, check.defaultManageCRDs(), "gatewayAPI", "manageCRDs")
			require.Equal(t, tt.wantManageCRDs, nestedValue(values, "gatewayAPI", "manageCRDs"))

			warning, err := check.validate(values)
			require.Equal(t, tt.wantWarning, warning != "", "warning: %q", warning)
			if len(tt.wantErr) == 0 {
				require.NoError(t, err)
				return
			}

			require.Error(t, err)
			require.True(t, clierrors.IsFriendlyError(err), "the CLI must show the message without a stack of wrapped errors")
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
// Contour arguments that make the check pass.
func Test_gatewayAPICRDCheck_SuggestedFlagTurnsOffFeatures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		values   map[string]any
		wantArgs []any
	}{
		{
			name: "no Contour arguments",
			wantArgs: []any{
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
		{
			name: "existing Contour arguments are kept",
			values: map[string]any{
				"contour": map[string]any{
					"extraArgs": []any{`--root-namespaces=team-a,team-b`, `--debug={x}\y`, "--disable-feature=tlsroutes"},
				},
			},
			wantArgs: []any{
				`--root-namespaces=team-a,team-b`,
				`--debug={x}\y`,
				"--disable-feature=tlsroutes",
				"--disable-feature=tcproutes",
				"--disable-feature=backendtlspolicies",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			support := contourGatewayAPISupportByVersion["1.32.0"]
			check := gatewayAPICRDCheck{
				releaseName:      contourReleaseName,
				releaseNamespace: RadiusSystemNamespace,
				contourVersion:   "1.32.0",
				support:          &support,
				crds:             traefikGatewayAPICRDs(),
			}
			values := copyValues(tt.values)
			setDefaultValue(values, check.defaultManageCRDs(), "gatewayAPI", "manageCRDs")
			_, err := check.validate(values)
			require.Error(t, err)

			match := regexp.MustCompile(`--contour-set '([^']*)'`).FindStringSubmatch(err.Error())
			require.Len(t, match, 2, "the error must suggest a --contour-set flag: %s", err)

			suggested := map[string]any{}
			require.NoError(t, strvals.ParseInto(match[1], suggested))
			require.Equal(t, tt.wantArgs, nestedValue(suggested, "contour", "extraArgs"))

			// Like --contour-set, the suggested value overrides the existing values and makes the check pass.
			require.NoError(t, strvals.ParseInto(match[1], values))
			_, err = check.validate(values)
			require.NoError(t, err)
		})
	}
}

func Test_contourArgsFromValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		values       map[string]any
		wantKey      string
		wantFeatures []string
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
			wantKey:      "contour.extraArgs",
			wantFeatures: []string{"tlsroutes", "tcproutes"},
		},
		{
			name: "contour.args replaces the chart's arguments",
			values: map[string]any{"contour": map[string]any{
				"args":      []any{"serve", "--disable-feature=grpcroutes"},
				"extraArgs": []any{"--disable-feature=tlsroutes"},
			}},
			wantKey:      "contour.args",
			wantFeatures: []string{"grpcroutes"},
		},
		{
			name: "empty contour.args does not replace the chart's arguments",
			values: map[string]any{"contour": map[string]any{
				"args":      []any{},
				"extraArgs": []any{"--disable-feature=tlsroutes"},
			}},
			wantKey:      "contour.extraArgs",
			wantFeatures: []string{"tlsroutes"},
		},
		{
			name:    "extraArgs that is not a list",
			values:  map[string]any{"contour": map[string]any{"extraArgs": "--disable-feature=tlsroutes"}},
			wantKey: "contour.extraArgs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := contourArgsFromValues(tt.values)
			require.Equal(t, tt.wantKey, args.key)
			require.Equal(t, tt.wantFeatures, args.disabledFeatures())
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
		name   string
		values map[string]any
		want   map[string]any
	}{
		{
			name:   "not set",
			values: map[string]any{},
			want:   map[string]any{"gatewayAPI": map[string]any{"manageCRDs": true}},
		},
		{
			name:   "parent table without the value",
			values: map[string]any{"gatewayAPI": map[string]any{"other": "x"}},
			want:   map[string]any{"gatewayAPI": map[string]any{"other": "x", "manageCRDs": true}},
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

			setDefaultValue(tt.values, true, "gatewayAPI", "manageCRDs")
			require.Equal(t, tt.want, tt.values)
		})
	}
}

func Test_gatewayAPICRDsInManifest(t *testing.T) {
	t.Parallel()

	manifest := `---
# Source: contour/templates/crds/gateway-api-crds.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: gateways.gateway.networking.k8s.io
spec:
  group: gateway.networking.k8s.io
---
# Source: contour/templates/crds/contour-crds.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: httpproxies.projectcontour.io
spec:
  group: projectcontour.io
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
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: httproutes.gateway.networking.k8s.io
spec:
  group: gateway.networking.k8s.io
`

	names, err := gatewayAPICRDsInManifest(manifest)
	require.NoError(t, err)
	require.Equal(t, []string{"gateways.gateway.networking.k8s.io", "httproutes.gateway.networking.k8s.io"}, names)

	names, err = gatewayAPICRDsInManifest("")
	require.NoError(t, err)
	require.Empty(t, names)

	_, err = gatewayAPICRDsInManifest("kind: [")
	require.Error(t, err)
}

func Test_listGatewayAPICRDsWithClient(t *testing.T) {
	t.Parallel()

	newCRD := func(name string, group string, versions ...apiextensionsv1.CustomResourceDefinitionVersion) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Labels:      map[string]string{helmManagedByLabel: helmManagedByValue},
				Annotations: map[string]string{helmReleaseNameAnnotation: "traefik-crd", helmReleaseNamespaceAnnotation: "kube-system"},
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

	crds := []apiextensionsv1.CustomResourceDefinition{
		*newCRD("backendtlspolicies.gateway.networking.k8s.io", gatewayAPIGroup, served("v1"), notServed("v1alpha3")),
		*newCRD("httpproxies.projectcontour.io", "projectcontour.io", served("v1")),
		*newCRD("httproutes.gateway.networking.k8s.io", gatewayAPIGroup, served("v1"), served("v1beta1")),
		*newCRD("xlistenersets.gateway.networking.x-k8s.io", "gateway.networking.x-k8s.io", served("v1alpha1")),
	}

	client := apiextensionsfake.NewClientset()
	var pageRequests []metav1.ListOptions
	// Serve one CRD per page so that the list follows continue tokens.
	client.PrependReactor("list", "customresourcedefinitions", func(action k8stesting.Action) (bool, runtime.Object, error) {
		options := action.(k8stesting.ListActionImpl).GetListOptions()
		pageRequests = append(pageRequests, options)

		index := 0
		if options.Continue != "" {
			var err error
			index, err = strconv.Atoi(options.Continue)
			require.NoError(t, err)
		}
		page := &apiextensionsv1.CustomResourceDefinitionList{Items: crds[index : index+1]}
		if index+1 < len(crds) {
			page.Continue = strconv.Itoa(index + 1)
		}
		return true, page, nil
	})

	got, err := listGatewayAPICRDsWithClient(t.Context(), client)
	require.NoError(t, err)
	require.Len(t, pageRequests, len(crds))
	for _, request := range pageRequests {
		require.Equal(t, int64(crdListPageSize), request.Limit)
	}

	require.Equal(t, []gatewayAPICRD{
		newHelmOwnedCRD("traefik-crd", "kube-system", "backendtlspolicies", "v1"),
		newHelmOwnedCRD("traefik-crd", "kube-system", "httproutes", "v1", "v1beta1"),
	}, got)
}

func Test_listGatewayAPICRDsWithClient_Error(t *testing.T) {
	t.Parallel()

	client := apiextensionsfake.NewClientset()
	client.PrependReactor("list", "customresourcedefinitions", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})

	_, err := listGatewayAPICRDsWithClient(t.Context(), client)
	require.ErrorContains(t, err, "forbidden")
}

func Test_gatewayAPICRD_owner(t *testing.T) {
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

func newGatewayAPICRD(resource string, labels map[string]string, annotations map[string]string, servedVersions ...string) gatewayAPICRD {
	return gatewayAPICRD{
		name:           resource + "." + gatewayAPIGroup,
		labels:         labels,
		annotations:    annotations,
		servedVersions: servedVersions,
	}
}

func newHelmOwnedCRD(releaseName string, releaseNamespace string, resource string, servedVersions ...string) gatewayAPICRD {
	return newGatewayAPICRD(resource,
		map[string]string{helmManagedByLabel: helmManagedByValue},
		map[string]string{helmReleaseNameAnnotation: releaseName, helmReleaseNamespaceAnnotation: releaseNamespace},
		servedVersions...)
}

func newUnmanagedCRD(resource string, servedVersions ...string) gatewayAPICRD {
	return newGatewayAPICRD(resource, nil, nil, servedVersions...)
}

// traefikGatewayAPICRDs returns the standard channel Gateway API v1.4.0 CRDs that k3s v1.35 installs with its
// traefik-crd Helm release. They do not include TLSRoute or TCPRoute, and BackendTLSPolicy serves only v1.
func traefikGatewayAPICRDs() []gatewayAPICRD {
	crd := func(resource string, servedVersions ...string) gatewayAPICRD {
		return newHelmOwnedCRD("traefik-crd", "kube-system", resource, servedVersions...)
	}
	return []gatewayAPICRD{
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
func contourBundleCRDs() []gatewayAPICRD {
	crd := func(resource string, servedVersions ...string) gatewayAPICRD {
		return newHelmOwnedCRD(contourReleaseName, RadiusSystemNamespace, resource, servedVersions...)
	}
	return []gatewayAPICRD{
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

func contourBundleCRDNames() []string {
	var names []string
	for _, crd := range contourBundleCRDs() {
		names = append(names, crd.name)
	}
	return names
}

// unmanagedCRDs returns crds without ownership metadata, as kubectl apply installs them.
func unmanagedCRDs(crds []gatewayAPICRD) []gatewayAPICRD {
	for i := range crds {
		crds[i].labels = nil
		crds[i].annotations = nil
	}
	return crds
}
