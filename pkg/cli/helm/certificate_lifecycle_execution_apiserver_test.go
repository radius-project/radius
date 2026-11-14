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
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

func TestCertificateSelectedMutationNativeAdmission(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Set KUBEBUILDER_ASSETS for isolated lifecycle mutation tests.")
	}
	kubeconfig, kube := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	store := &rotationStore{kube: kube, dynamic: client, namespace: RadiusSystemNamespace, release: radiusReleaseName, now: time.Now}
	installRotationTestCRDs(t, store)
	w := lifecycleExecutionFixtureInStore(t, store)
	// Only API admission, CAS and recovery are real here. This fixture neither
	// runs cert-manager/CSI nor proves workload trust adoption.
	w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
	resource, err := (certificateMutationIntent{Kind: "Deployment", Name: "ucp"}).resource()
	require.NoError(t, err)
	desired := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "ucp", "namespace": store.namespace,
			"labels":      map[string]any{helmManagedByLabel: helmManagedByValue},
			"annotations": map[string]any{helmReleaseNameAnnotation: store.release, helmReleaseNamespaceAnnotation: store.namespace}},
		"spec": map[string]any{"replicas": int64(2),
			"selector": map[string]any{"matchLabels": map[string]any{"app": "ucp"}},
			"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "ucp"}},
				"spec": map[string]any{"containers": []any{map[string]any{"name": "ucp", "image": "fixture"}}}}},
	}}
	before, err := client.Resource(resource).Namespace(store.namespace).Create(t.Context(), desired, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(desired.Object, "selected", "spec", "template", "metadata", "annotations", "checksum/trust"))
	require.NoError(t, withIdentityOperation(t.Context(), kube, store.namespace, func(operation *identityOperation) error {
		w.operation = operation
		require.NoError(t, w.save(t.Context(), w.state))
		for _, name := range []string{"internal", "aggregation"} {
			require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/publish"))
		}
		convergeAPIServiceFixture(t, w)
		executor := &certificateLifecycleExecutor{writer: w}
		require.NoError(t, executor.advance(t.Context(), rotationPublish))
		require.NoError(t, executor.advance(t.Context(), rotationAdopt))
		admitted, err := w.admitSelectedMutation(t.Context(), desired)
		require.NoError(t, err)
		live, err := client.Resource(resource).Namespace(store.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		require.Equal(t, before.GetResourceVersion(), live.GetResourceVersion(), "dry-run must not mutate")
		require.NoError(t, w.applySelectedMutation(t.Context(), admitted))
		resumeExecutionFixture(t, w)
		retry, err := w.admitSelectedMutation(t.Context(), desired)
		require.NoError(t, err)
		version := w.journal.ResourceVersion
		require.NoError(t, w.applySelectedMutation(t.Context(), retry))
		require.Equal(t, version, w.journal.ResourceVersion)
		live, err = client.Resource(resource).Namespace(store.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
		require.NoError(t, err)
		replicas, _, err := unstructured.NestedInt64(live.Object, "spec", "replicas")
		require.NoError(t, err)
		require.EqualValues(t, 2, replicas)
		return nil
	}))
}
