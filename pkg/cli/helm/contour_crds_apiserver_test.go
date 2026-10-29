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
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/radius-project/radius/pkg/cli/clierrors"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"
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

	t.Run("k3s cluster with Traefik", func(t *testing.T) {
		kubeconfig, crdClient := startGatewayAPITestServer(t)
		traefikCRDs := traefikGatewayAPICRDs()
		for _, crd := range traefikCRDs {
			_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(t.Context(), newTestCRD(t, crd), metav1.CreateOptions{})
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
		require.Contains(t, err.Error(), "--contour-set '"+turnOffFeaturesForTraefikCRDs+"'")
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
		require.ErrorContains(t, err, "Add this flag to the rad upgrade kubernetes command:\n      --contour-set '"+turnOffFeaturesForTraefikCRDs+"'")
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
		require.ErrorContains(t, err, "make Helm delete these CRDs")
		require.Equal(t, 2, getTestRelease(t, kubeconfig, contourReleaseName).Version)
		requireCRDsUnchanged(t, crdClient, contourCRDs)
	})
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

// writeStubCharts writes a Radius chart and a Contour chart that render a ConfigMap. Like the real Contour chart, the
// Contour chart renders the Gateway API CRDs that Contour 1.32.0 bundles only when gatewayAPI.manageCRDs is true.
func writeStubCharts(t *testing.T) (string, string) {
	t.Helper()

	radiusChart := filepath.Join(t.TempDir(), "radius")
	writeTestFile(t, filepath.Join(radiusChart, "Chart.yaml"), "apiVersion: v2\nname: radius\nversion: 0.1.0\n")
	writeTestFile(t, filepath.Join(radiusChart, "templates", "configmap.yaml"),
		"apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: radius-stub\ndata:\n  installed: \"true\"\n")

	var crds strings.Builder
	crds.WriteString("{{- if .Values.gatewayAPI.manageCRDs }}\n")
	for _, crd := range contourBundleCRDs() {
		crd.labels = nil
		crd.annotations = nil
		document, err := yaml.Marshal(newTestCRD(t, crd))
		require.NoError(t, err)
		crds.WriteString("---\n")
		crds.Write(document)
	}
	crds.WriteString("{{- end }}\n")

	contourChart := filepath.Join(t.TempDir(), "contour")
	writeTestFile(t, filepath.Join(contourChart, "Chart.yaml"), "apiVersion: v2\nname: contour\nversion: 0.1.0\nappVersion: 1.32.0\n")
	writeTestFile(t, filepath.Join(contourChart, "values.yaml"), "contour:\n  extraArgs: []\ngatewayAPI:\n  manageCRDs: false\n")
	writeTestFile(t, filepath.Join(contourChart, "templates", "crds", "gateway-api-crds.yaml"), crds.String())
	writeTestFile(t, filepath.Join(contourChart, "templates", "configmap.yaml"), `apiVersion: v1
kind: ConfigMap
metadata:
  name: contour-stub
data:
  manageCRDs: {{ .Values.gatewayAPI.manageCRDs | quote }}
  extraArgs: {{ .Values.contour.extraArgs | toJson | quote }}
`)

	return radiusChart, contourChart
}

func stubClusterOptions(radiusChart string, contourChart string) ClusterOptions {
	options := NewDefaultClusterOptions()
	options.Radius.ChartPath = radiusChart
	options.Radius.Timeout = time.Minute
	options.Contour.ChartPath = contourChart
	options.Contour.Timeout = time.Minute
	return options
}

// newTestCRD returns a CRD with the name, ownership metadata, and served versions of crd.
func newTestCRD(t *testing.T, crd gatewayAPICRD) *apiextensionsv1.CustomResourceDefinition {
	t.Helper()

	kinds := map[string]string{
		"backendlbpolicies":  "BackendLBPolicy",
		"backendtlspolicies": "BackendTLSPolicy",
		"gatewayclasses":     "GatewayClass",
		"gateways":           "Gateway",
		"grpcroutes":         "GRPCRoute",
		"httproutes":         "HTTPRoute",
		"referencegrants":    "ReferenceGrant",
		"tcproutes":          "TCPRoute",
		"tlsroutes":          "TLSRoute",
		"udproutes":          "UDPRoute",
	}
	plural := strings.TrimSuffix(crd.name, "."+gatewayAPIGroup)
	kind, found := kinds[plural]
	require.True(t, found, "add the kind of %s", crd.name)

	scope := apiextensionsv1.NamespaceScoped
	if kind == "GatewayClass" {
		scope = apiextensionsv1.ClusterScoped
	}
	// The API server requires this annotation for CRDs in *.k8s.io groups. The Gateway API CRDs have it.
	annotations := map[string]string{"api-approved.kubernetes.io": "https://github.com/kubernetes-sigs/gateway-api/pull/3328"}
	maps.Copy(annotations, crd.annotations)
	preserveUnknownFields := true
	object := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{APIVersion: "apiextensions.k8s.io/v1", Kind: "CustomResourceDefinition"},
		ObjectMeta: metav1.ObjectMeta{
			Name:        crd.name,
			Labels:      crd.labels,
			Annotations: annotations,
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gatewayAPIGroup,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   plural,
				Singular: strings.ToLower(kind),
				Kind:     kind,
				ListKind: kind + "List",
			},
			Scope: scope,
		},
	}
	for i, version := range crd.servedVersions {
		object.Spec.Versions = append(object.Spec.Versions, apiextensionsv1.CustomResourceDefinitionVersion{
			Name:    version,
			Served:  true,
			Storage: i == 0,
			Schema: &apiextensionsv1.CustomResourceValidation{
				OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &preserveUnknownFields},
			},
		})
	}

	return object
}

// requireCRDsUnchanged verifies that the cluster has exactly the Gateway API CRDs in want, with the same owners and
// served versions.
func requireCRDsUnchanged(t *testing.T, crdClient apiextensionsclient.Interface, want []gatewayAPICRD) {
	t.Helper()

	got, err := listGatewayAPICRDsWithClient(t.Context(), crdClient)
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
