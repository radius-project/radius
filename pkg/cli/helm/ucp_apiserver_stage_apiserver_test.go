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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	sigsyaml "sigs.k8s.io/yaml"
)

// TestStagedUCPAPIServerCAAdoptedByHelm_APIServer checks, against a real API server, that Helm v4 adopts the
// radius-apiserver-ca chain that rad applies before the Helm upgrade that turns authorization on: the upgrade
// succeeds without an ownership conflict, records the objects in the release, and deletes them when a later revision
// no longer renders them. An object without Helm's ownership metadata is refused by rad and by Helm alike.
func TestStagedUCPAPIServerCAAdoptedByHelm_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	installCertManagerTestCRDs(t, apiextclient.NewForConfigOrDie(config))

	staged, err := ucpAPIServerCAStage(renderStageTestManifest(t, authzOnValues))
	require.NoError(t, err)
	require.Len(t, staged, len(ucpAPIServerCAStageObjects))

	tests := []struct {
		name            string
		serverSideApply bool
	}{
		{name: "server-side apply release", serverSideApply: true},
		{name: "client-side apply release", serverSideApply: false},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := fmt.Sprintf("stage-%d", i)
			_, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
			require.NoError(t, err)
			configuration := helmTestConfiguration(t, kubeconfig, namespace)

			// Authorization is off: the release has no radius-apiserver-ca chain.
			install := newInstallClient(configuration, "radius", namespace, false, time.Minute)
			install.ServerSideApply = tt.serverSideApply
			_, err = install.Run(stagedCATestChart(t, nil, namespace), map[string]any{})
			require.NoError(t, err)

			for _, object := range staged {
				require.NoError(t, applyHelmOwnedObject(t.Context(), "test-context", dynamicClient, object.resource, object.object, "radius", namespace))
			}
			// A rerun after a partial stage updates the objects it created.
			for _, object := range staged {
				require.NoError(t, applyHelmOwnedObject(t.Context(), "test-context", dynamicClient, object.resource, object.object, "radius", namespace))
			}

			_, err = NewHelmClient().RunHelmUpgrade(configuration, stagedCATestChart(t, staged, namespace), map[string]any{}, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err, "Helm must adopt the staged objects")
			deployed, err := NewHelmClient().RunHelmGet(configuration, "radius")
			require.NoError(t, err)
			for _, object := range staged {
				require.Contains(t, deployed.Manifest, "name: "+object.object.GetName())
				live, err := dynamicClient.Resource(object.resource).Namespace(namespace).Get(t.Context(), object.object.GetName(), metav1.GetOptions{})
				require.NoError(t, err)
				want, err := json.Marshal(object.object.Object["spec"])
				require.NoError(t, err)
				got, err := json.Marshal(live.Object["spec"])
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got))
			}

			// The objects belong to the release: Helm deletes them once a revision no longer renders them.
			_, err = NewHelmClient().RunHelmUpgrade(configuration, stagedCATestChart(t, nil, namespace), map[string]any{}, "radius", namespace, false, true, time.Minute)
			require.NoError(t, err)
			for _, object := range staged {
				_, err := dynamicClient.Resource(object.resource).Namespace(namespace).Get(t.Context(), object.object.GetName(), metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "Helm must delete %s it adopted, got %v", object.object.GetName(), err)
			}
		})
	}

	t.Run("object without Helm ownership", func(t *testing.T) {
		namespace := "stage-unowned"
		_, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		require.NoError(t, err)
		configuration := helmTestConfiguration(t, kubeconfig, namespace)
		_, err = newInstallClient(configuration, "radius", namespace, false, time.Minute).Run(stagedCATestChart(t, nil, namespace), map[string]any{})
		require.NoError(t, err)

		unowned := staged[0].object.DeepCopy()
		unowned.SetNamespace(namespace)
		_, err = dynamicClient.Resource(staged[0].resource).Namespace(namespace).Create(t.Context(), unowned, metav1.CreateOptions{})
		require.NoError(t, err)

		err = applyHelmOwnedObject(t.Context(), "test-context", dynamicClient, staged[0].resource, staged[0].object, "radius", namespace)
		require.ErrorContains(t, err, "is not part of Helm release")
		require.ErrorContains(t, err, `kubectl --context "test-context" -n `+namespace+" delete issuer "+unowned.GetName())

		_, err = NewHelmClient().RunHelmUpgrade(configuration, stagedCATestChart(t, staged, namespace), map[string]any{}, "radius", namespace, false, true, time.Minute)
		require.ErrorContains(t, err, "cannot be imported into the current release")
	})

	// An upgrade that stops after rad stages the chain but before Helm adopts it leaves the chain outside the release,
	// so Helm's uninstall keeps it. rad deletes what it staged and leaves an object that another owner created.
	t.Run("uninstall before adoption", func(t *testing.T) {
		namespace := "stage-uninstall"
		_, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		require.NoError(t, err)
		configuration := helmTestConfiguration(t, kubeconfig, namespace)
		_, err = newInstallClient(configuration, "radius", namespace, false, time.Minute).Run(stagedCATestChart(t, nil, namespace), map[string]any{})
		require.NoError(t, err)

		foreign := staged[0].object.DeepCopy()
		foreign.SetNamespace(namespace)
		_, err = dynamicClient.Resource(staged[0].resource).Namespace(namespace).Create(t.Context(), foreign, metav1.CreateOptions{})
		require.NoError(t, err)
		for _, object := range staged[1:] {
			require.NoError(t, applyHelmOwnedObject(t.Context(), "test-context", dynamicClient, object.resource, object.object, "radius", namespace))
		}

		_, err = NewHelmClient().RunHelmUninstall(configuration, "radius", namespace, true)
		require.NoError(t, err)
		for _, object := range staged {
			_, err := dynamicClient.Resource(object.resource).Namespace(namespace).Get(t.Context(), object.object.GetName(), metav1.GetOptions{})
			require.NoError(t, err, "Helm's uninstall keeps %s, which is not in the release", object.object.GetName())
		}

		var logs []string
		impl := &Impl{logger: func(format string, v ...any) { logs = append(logs, fmt.Sprintf(format, v...)) }}
		for range 2 {
			logs = nil
			require.NoError(t, impl.deleteUnadoptedUCPAPIServerStage(t.Context(), "test-context", dynamicClient, namespace, "radius", nil, false))
			_, err = dynamicClient.Resource(staged[0].resource).Namespace(namespace).Get(t.Context(), foreign.GetName(), metav1.GetOptions{})
			require.NoError(t, err, "rad must keep %s, which it did not stage", foreign.GetName())
			for _, object := range staged[1:] {
				_, err := dynamicClient.Resource(object.resource).Namespace(namespace).Get(t.Context(), object.object.GetName(), metav1.GetOptions{})
				require.True(t, apierrors.IsNotFound(err), "rad must delete %s, which it staged, got %v", object.object.GetName(), err)
			}
			require.Len(t, logs, 1)
			require.Contains(t, logs[0], "kubectl --context \"test-context\" -n "+namespace+" delete issuer "+foreign.GetName())
		}

		// A delete pinned to an earlier resourceVersion fails against the real API server, so an object that changed
		// after rad read it survives.
		require.NoError(t, applyHelmOwnedObject(t.Context(), "test-context", dynamicClient, staged[1].resource, staged[1].object, "radius", namespace))
		live, err := dynamicClient.Resource(staged[1].resource).Namespace(namespace).Get(t.Context(), staged[1].object.GetName(), metav1.GetOptions{})
		require.NoError(t, err)
		uid, stale := live.GetUID(), live.GetResourceVersion()
		live.SetLabels(map[string]string{helmManagedByLabel: helmManagedByValue, "changed": "true"})
		_, err = dynamicClient.Resource(staged[1].resource).Namespace(namespace).Update(t.Context(), live, metav1.UpdateOptions{})
		require.NoError(t, err)
		err = dynamicClient.Resource(staged[1].resource).Namespace(namespace).Delete(t.Context(), staged[1].object.GetName(),
			metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &stale}})
		require.True(t, apierrors.IsConflict(err), "got %v", err)
	})
}

// lookupRenderCluster renders with a configuration whose template lookups read a real API server.
type lookupRenderCluster struct {
	*fakeContourCluster
	configuration func() *helm.Configuration
}

func (c lookupRenderCluster) RenderConfiguration(string, string) (*helm.Configuration, error) {
	return c.configuration(), nil
}

// TestRenderRadiusRollbackKeepsLiveUCPCert_APIServer checks, against a real API server, that rad stages what
// RunHelmRollback applies: an upgrade to the target chart and values whose lookups keep the live Secret ucp-cert.
// The target revision stored a different ucp-cert; staging it would replace the key UCP serves with.
func TestRenderRadiusRollbackKeepsLiveUCPCert_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, client := startTestAPIServer(t)
	namespace := RadiusSystemNamespace
	_, err := client.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	require.NoError(t, err)

	live := map[string][]byte{
		corev1.TLSCertKey:       []byte("live-certificate"),
		corev1.TLSPrivateKeyKey: []byte("live-key"),
		caCertificateKey:        []byte("live-ca"),
	}
	_, err = client.CoreV1().Secrets(namespace).Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: ucpOffCertificateSecretName, Namespace: namespace},
		Data:       live,
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	stored := renderStageTestManifest(t, map[string]any{})
	storedSecret, _ := renderedOffCertificate(t, stored)
	require.NotEqual(t, live[corev1.TLSPrivateKeyKey], storedSecret.Data[corev1.TLSPrivateKeyKey])
	history := []*releasev1.Release{
		{Name: radiusReleaseName, Namespace: namespace, Version: 1, Config: map[string]any{}, Manifest: stored, Chart: loadStageTestChart(t), Info: &releasev1.Info{Status: releasecommon.StatusSuperseded}},
		{Name: radiusReleaseName, Namespace: namespace, Version: 2, Config: authzOnValues, Manifest: renderStageTestManifest(t, authzOnValues), Chart: loadStageTestChart(t), Info: &releasev1.Info{Status: releasecommon.StatusDeployed}},
	}
	_, err = client.CoreV1().ConfigMaps(namespace).Create(t.Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "ucp-config", Namespace: namespace,
			Labels: map[string]string{helmManagedByLabel: helmManagedByValue},
			Annotations: map[string]string{helmReleaseNameAnnotation: radiusReleaseName,
				helmReleaseNamespaceAnnotation: namespace, "radius.io/tls-provider": "radius"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	impl := &Impl{contourCluster: lookupRenderCluster{
		fakeContourCluster: &fakeContourCluster{},
		configuration: func() *helm.Configuration {
			configuration := helmTestConfiguration(t, kubeconfig, namespace)
			configuration.KubeClient = &kubefake.PrintingKubeClient{Out: io.Discard}
			return configuration
		},
	}}

	manifest, ok, err := impl.renderRadiusRollback("test-context", namespace, history, 1)(t.Context())
	require.NoError(t, err)
	require.True(t, ok)
	rendered, _ := renderedOffCertificate(t, manifest)
	require.Equal(t, live, rendered.Data, "the rollback must render the live Secret %s, not the one revision 1 stored", ucpOffCertificateSecretName)
}

func helmTestConfiguration(t *testing.T, kubeconfig, namespace string) *helm.Configuration {
	t.Helper()
	flags := genericclioptions.NewConfigFlags(false)
	flags.KubeConfig = &kubeconfig
	flags.Namespace = &namespace
	configuration := &helm.Configuration{}
	require.NoError(t, configuration.Init(flags, namespace, helmDriverSecret))
	return configuration
}

// stagedCATestChart is a chart named radius with a ConfigMap and, as templates, the staged objects in namespace.
func stagedCATestChart(t *testing.T, staged []stagedObject, namespace string) *chart.Chart {
	t.Helper()
	chartPath := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(chartPath, "templates"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "Chart.yaml"), []byte("apiVersion: v2\nname: radius\nversion: 0.1.0\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "configmap.yaml"),
		[]byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: radius\ndata: {}\n"), 0600))
	documents := []string{}
	for _, object := range staged {
		rendered := object.object.DeepCopy()
		rendered.SetNamespace(namespace)
		document, err := sigsyaml.Marshal(rendered.Object)
		require.NoError(t, err)
		documents = append(documents, string(document))
	}
	require.NoError(t, os.WriteFile(filepath.Join(chartPath, "templates", "certificates.yaml"), []byte(strings.Join(documents, "---\n")), 0600))
	c, err := loader.Load(chartPath)
	require.NoError(t, err)
	return c
}

// installCertManagerTestCRDs installs the Certificate and Issuer CRDs with schemas that accept any spec, which is
// all Helm's adoption needs.
func installCertManagerTestCRDs(t *testing.T, client apiextclient.Interface) {
	t.Helper()
	preserve := true
	for _, kind := range []string{"Certificate", "Issuer"} {
		plural := strings.ToLower(kind) + "s"
		_, err := client.ApiextensionsV1().CustomResourceDefinitions().Create(t.Context(), &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: plural + ".cert-manager.io"},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: "cert-manager.io",
				Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: kind, ListKind: kind + "List", Plural: plural, Singular: strings.ToLower(kind)},
				Scope: apiextensionsv1.NamespaceScoped,
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
					Name: "v1", Served: true, Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object", XPreserveUnknownFields: &preserve,
					}},
				}},
			},
		}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, plural := range []string{"certificates", "issuers"} {
			crd, err := client.ApiextensionsV1().CustomResourceDefinitions().Get(t.Context(), plural+".cert-manager.io", metav1.GetOptions{})
			if !assert.NoError(c, err) {
				return
			}
			established := false
			for _, condition := range crd.Status.Conditions {
				established = established || (condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue)
			}
			assert.True(c, established, "%s is not established", crd.Name)
		}
	}, 30*time.Second, 100*time.Millisecond)
}
