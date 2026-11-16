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
	"encoding/base64"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// A serialized legacy marker tests rejection without retaining a retired writer.
func newLegacyRotationTest(t *testing.T) (*rotationStore, *rest.Config, *corev1.ConfigMap) {
	s, config, _, _ := newRotationTestStore(t)
	journal, err := s.kube.CoreV1().ConfigMaps(s.namespace).Create(t.Context(), &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: rotationJournalName, Namespace: s.namespace, Labels: map[string]string{rotationOwner: s.release}},
		Data:       map[string]string{rotationJournalKey: `{"schemaVersion":1,"phase":"publish"}`},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	api, err := s.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	annotations := api.GetAnnotations()
	annotations[rotationJournalUID], annotations[rotationRevision] = string(journal.UID), "legacy/publish"
	api.SetAnnotations(annotations)
	_, err = s.dynamic.Resource(apiServiceResource).Update(t.Context(), api, metav1.UpdateOptions{})
	require.NoError(t, err)
	return s, config, journal
}

func newRotationTestStore(t *testing.T) (*rotationStore, *rest.Config, []byte, []byte) {
	t.Helper()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated CA rotation storage tests.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	old, _, _ := testSigningCA(t, nil, nil, nil, nil)
	next, _, _ := testSigningCA(t, nil, nil, nil, nil)
	_, err = client.Resource(apiServiceResource).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{
			"name": radiusAPIService,
			"annotations": map[string]any{
				"meta.helm.sh/release-name": radiusReleaseName, "meta.helm.sh/release-namespace": RadiusSystemNamespace,
			},
		},
		"spec": map[string]any{
			"group": "api.ucp.dev", "version": "v1alpha3", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp"},
			"caBundle": base64.StdEncoding.EncodeToString(old),
		},
	}}, metav1.CreateOptions{FieldManager: FieldManager})
	require.NoError(t, err)
	return &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName,
		now: time.Now}, config, old, next
}

type rotationRoundTripper func(*http.Request) (*http.Response, error)

func (f rotationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
