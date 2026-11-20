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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
)

// TestAPIServiceRetirementDoesNotFenceQueuedInjectorApply reproduces the force-SSA
// write shape in cert-manager v1.21.2's cainjector reconciler, not a running injector.
// Changing the source and removing the annotation do not cancel a computed write.
func TestAPIServiceRetirementDoesNotFenceQueuedInjectorApply(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS to run the isolated APIService stale-apply regression.")
	}
	runtimelog.SetLogger(logr.Discard())
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	services := client.Resource(schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices",
	})
	oldCA, _, _ := testSigningCA(t, nil, nil, nil, nil)
	newCA, _, _ := testSigningCA(t, nil, nil, nil, nil)
	overlap := append(bytes.Clone(oldCA), newCA...)
	_, err = kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	secrets := kube.CoreV1().Secrets(RadiusSystemNamespace)
	source, err := secrets.Create(t.Context(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "radius-aggregation-ca", Annotations: map[string]string{
			"cert-manager.io/allow-direct-injection": "true",
		}},
		Data: map[string][]byte{"ca.crt": overlap},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	const name = "v1alpha3.api.ucp.dev"
	live, err := services.Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": name, "annotations": map[string]any{
			"cert-manager.io/inject-ca-from-secret": RadiusSystemNamespace + "/" + source.Name,
		}},
		"spec": map[string]any{
			"group": "api.ucp.dev", "version": "v1alpha3", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": RadiusSystemNamespace, "name": "ucp"},
			"caBundle": base64.StdEncoding.EncodeToString(overlap),
		},
	}}, metav1.CreateOptions{FieldManager: "rad"})
	require.NoError(t, err)
	queued := map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": name},
		"spec":     map[string]any{"caBundle": base64.StdEncoding.EncodeToString(overlap)},
	}
	staleResourceVersion := live.GetResourceVersion()
	source.Data["ca.crt"] = newCA
	_, err = secrets.Update(t.Context(), source, metav1.UpdateOptions{})
	require.NoError(t, err)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := services.Get(t.Context(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		current.SetAnnotations(nil)
		if err := unstructured.SetNestedField(current.Object, base64.StdEncoding.EncodeToString(newCA), "spec", "caBundle"); err != nil {
			return err
		}
		live, err = services.Update(t.Context(), current, metav1.UpdateOptions{FieldManager: "rad"})
		return err
	})
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(newCA), live.Object["spec"].(map[string]any)["caBundle"])
	require.Empty(t, live.GetAnnotations())

	stale, err := json.Marshal(queued)
	require.NoError(t, err)
	force := true
	live, err = services.Patch(t.Context(), name, types.ApplyPatchType, stale,
		metav1.PatchOptions{FieldManager: "cert-manager-cainjector", Force: &force})
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(overlap), live.Object["spec"].(map[string]any)["caBundle"],
		"a queued injector write can restore a still-valid retired root after a successful CAS")
	require.Empty(t, live.GetAnnotations(), "the annotation is not a server-enforced write fence")

	// A resourceVersion precondition would reject that stale write, but the
	// supported injector does not include one. Radius cannot add it on its behalf.
	queued["metadata"].(map[string]any)["resourceVersion"] = staleResourceVersion
	stale, err = json.Marshal(queued)
	require.NoError(t, err)
	_, err = services.Patch(t.Context(), name, types.ApplyPatchType, stale,
		metav1.PatchOptions{FieldManager: "cert-manager-cainjector", Force: &force})
	require.True(t, apierrors.IsConflict(err), "a stale precondition must fail even with force apply: %v", err)
}
