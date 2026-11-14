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
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCertificateSelectedMutationRecovery(t *testing.T) {
	for _, scenario := range []string{"lost response", "changed retry", "observer unavailable", "canceled", "status update", "metadata update"} {
		t.Run(scenario, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			// Injected observation fixture; no workload or authenticated route runs.
			w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
			resource, err := (certificateMutationIntent{Kind: "Deployment", Name: "ucp"}).resource()
			require.NoError(t, err)
			before, err := w.store.dynamic.Resource(resource).Namespace(w.store.namespace).Create(t.Context(), &unstructured.Unstructured{
				Object: map[string]any{"apiVersion": "apps/v1", "kind": "Deployment",
					"metadata": map[string]any{"name": "ucp", "namespace": w.store.namespace,
						"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
						"annotations": map[string]any{helmReleaseNameAnnotation: w.store.release,
							helmReleaseNamespaceAnnotation: w.store.namespace}},
					"spec": map[string]any{"replicas": int64(1), "selector": map[string]any{"matchLabels": map[string]any{"app": "ucp"}},
						"template": map[string]any{"metadata": map[string]any{"labels": map[string]any{"app": "ucp"}},
							"spec": map[string]any{"containers": []any{map[string]any{"name": "ucp", "image": "fixture",
								"env": []any{map[string]any{"name": "FIXTURE_SECRET", "value": "never-copy-this-fixture-value"}}}}}}}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			desired := before.DeepCopy()
			require.NoError(t, unstructured.SetNestedField(desired.Object, "selected", "spec", "template", "metadata", "annotations", "checksum/trust"))
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				for _, name := range []string{"internal", "aggregation"} {
					require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/publish"))
				}
				convergeAPIServiceFixture(t, w)
				for _, phase := range []rotationPhase{rotationPublish, rotationAdopt} {
					next, err := cloneCertificateLifecycle(w.state)
					require.NoError(t, err)
					next.Phase = phase
					require.NoError(t, w.save(t.Context(), next))
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if scenario == "observer unavailable" {
					w.observeLoaded = nil
				} else if scenario == "canceled" {
					cancel()
				}
				if scenario == "lost response" {
					fail := true
					w.store.kube.(*kubefake.Clientset).PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
						object := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
						if !fail || object.Name != rotationJournalName {
							return false, nil, nil
						}
						state, err := decodeCertificateLifecycle(object, w.store.namespace, w.store.release, w.store.now())
						require.NoError(t, err)
						if len(state.Execution.Mutations) == 0 || state.Execution.Mutations[0].Observed == nil {
							return false, nil, nil
						}
						fail = false
						return true, nil, errors.New("fixture interruption after workload CAS")
					})
				}
				if scenario == "status update" || scenario == "metadata update" {
					change := true
					w.store.kube.(*kubefake.Clientset).PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
						record := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
						if !change || record.Name != rotationJournalName {
							return false, nil, nil
						}
						change = false
						tracker := w.store.dynamic.(*dynamicfake.FakeDynamicClient).Tracker()
						live, err := tracker.Get(resource, w.store.namespace, "ucp")
						require.NoError(t, err)
						object := live.(*unstructured.Unstructured)
						object.SetResourceVersion("fixture-status-revision")
						object.Object["status"] = map[string]any{"readyReplicas": int64(1)}
						if scenario == "metadata update" {
							annotations := object.GetAnnotations()
							annotations["other-writer"] = "changed"
							object.SetAnnotations(annotations)
						}
						require.NoError(t, tracker.Update(resource, object, w.store.namespace))
						return false, nil, nil
					})
				}
				err := w.applySelectedMutation(ctx, desired)
				switch scenario {
				case "metadata update":
					require.ErrorContains(t, err, "exact before or owned after")
					return nil
				case "observer unavailable", "canceled":
					require.Error(t, err)
					require.Empty(t, w.state.Execution.Mutations)
					after, err := w.store.dynamic.Resource(resource).Namespace(w.store.namespace).Get(t.Context(), "ucp", metav1.GetOptions{})
					require.NoError(t, err)
					require.Equal(t, before, after)
					return nil
				case "lost response":
					require.ErrorContains(t, err, "fixture interruption")
					resumeExecutionFixture(t, w)
					require.NoError(t, w.applySelectedMutation(t.Context(), desired))
				default:
					require.NoError(t, err)
				}
				require.NotContains(t, w.journal.Data[rotationJournalKey], "never-copy-this-fixture-value")
				version := w.journal.ResourceVersion
				require.NoError(t, w.applySelectedMutation(t.Context(), desired))
				require.Equal(t, version, w.journal.ResourceVersion)
				if scenario == "changed retry" {
					require.NoError(t, unstructured.SetNestedField(desired.Object, "changed", "spec", "template", "metadata", "annotations", "checksum/trust"))
					require.ErrorContains(t, w.applySelectedMutation(t.Context(), desired), "re-rendered workload differs")
				}
				return nil
			}))
		})
	}
}
