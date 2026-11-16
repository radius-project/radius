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
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCertificatePreparationSelectedChart(t *testing.T) {
	for _, scope := range []string{"", "internal", "aggregation"} {
		t.Run(scope, func(t *testing.T) {
			b, _ := initialControllerFixture(t, "selfSigned", "radius")
			require.NoError(t, b.run(t.Context()))
			deployment, replicaSet, pod := injectorFixture("cert-manager-cainjector")
			for _, object := range []runtime.Object{deployment, replicaSet, pod} {
				require.NoError(t, b.store.kube.(*kubefake.Clientset).Tracker().Add(object))
			}
			target, err := certificateProfilePlan(b.selected, b.values, b.store.namespace)
			require.NoError(t, err)
			for name, plane := range target.Planes {
				signingSecret := "radius-ca"
				if name == "aggregation" {
					signingSecret = "radius-apiserver-ca"
				}
				_, err = b.store.dynamic.Resource(rotationIssuerResource).Namespace(b.store.namespace).Create(t.Context(), &unstructured.Unstructured{
					Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
						"metadata": map[string]any{"name": plane.Issuer.Name, "namespace": b.store.namespace,
							"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
							"annotations": map[string]any{helmReleaseNameAnnotation: b.store.release,
								helmReleaseNamespaceAnnotation: b.store.namespace}},
						"spec": map[string]any{"ca": map[string]any{"secretName": signingSecret}}},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			_, err = b.store.dynamic.Resource(apiServiceResource).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
				"metadata": map[string]any{"name": radiusAPIService,
					"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
					"annotations": map[string]any{helmReleaseNameAnnotation: b.store.release,
						helmReleaseNamespaceAnnotation: b.store.namespace,
						injectCAFromSecretAnnotation:   ucpAPIServerCASource(b.store.namespace)}},
				"spec": map[string]any{"service": map[string]any{"namespace": b.store.namespace, "name": "ucp"},
					"caBundle": base64.StdEncoding.EncodeToString(b.state.Anchors["aggregation"].CA)},
			}}, metav1.CreateOptions{})
			require.NoError(t, err)
			_, err = b.store.dynamic.Resource(rotationCertificateResource).Namespace(b.store.namespace).Create(t.Context(), &unstructured.Unstructured{
				Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
					"metadata": map[string]any{"name": "ucp-cert", "namespace": b.store.namespace,
						"labels":      map[string]any{helmManagedByLabel: helmManagedByValue},
						"annotations": map[string]any{helmReleaseNameAnnotation: b.store.release, helmReleaseNamespaceAnnotation: b.store.namespace}},
					"spec": map[string]any{"secretName": "radius-aggregation-previous"}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			installed := &releasev1.Release{Version: 1, Chart: b.selected, Config: copyValues(b.values)}
			unsupported := *target
			unsupported.Mode = "issuerRef"
			_, err = b.store.planCertificateLifecycle(t.Context(), installed, b.selected, &unsupported, "")
			require.ErrorContains(t, err, "staged CSI issuer configuration")
			unsupported = *target
			unsupported.TrustDomain = "changed.invalid"
			_, err = b.store.planCertificateLifecycle(t.Context(), installed, b.selected, &unsupported, scope)
			require.ErrorContains(t, err, "receiver inventory, trust domain or CSI driver")
			state, err := b.store.planCertificateLifecycle(t.Context(), installed, b.selected, target, scope)
			require.NoError(t, err)
			w := &certificateExecutionWriter{store: b.store, state: state}
			if scope == "internal" {
				require.Equal(t, "radius-aggregation-previous", state.Execution.ServingSecret)
				require.Equal(t, state.Execution.PreviousServingSecret, state.Execution.ServingSecret)
				require.NotNil(t, state.Execution.UnchangedAggregationLeaf)
			} else {
				require.NotEqual(t, state.Execution.PreviousServingSecret, state.Execution.ServingSecret)
			}
			kube := b.store.kube.(*kubefake.Clientset)
			// This fixture generates CA material and reports controller status.
			// No cert-manager, CSI, workload or protected observer runs here.
			kube.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
				name := action.(k8stesting.GetAction).GetName()
				var selected *certificateResourceIntent
				for _, intent := range w.state.Execution.Resources {
					if intent.Kind == "Certificate" && intent.Name == name {
						selected = &intent
					}
				}
				if selected == nil {
					return false, nil, nil
				}
				if _, err := kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), b.store.namespace, name); err == nil {
					return false, nil, nil
				}
				certificate, err := b.store.dynamic.Resource(rotationCertificateResource).Namespace(b.store.namespace).Get(t.Context(), name, metav1.GetOptions{})
				require.NoError(t, err)
				ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
				secret := caTLSSecret(name, ca, key)
				secret.Namespace = b.store.namespace
				secret.UID, secret.ResourceVersion = "fixture-"+certificate.GetUID(), "1"
				secret.Annotations = map[string]string{"cert-manager.io/issuer-name": RadiusBootstrapIssuerName,
					"cert-manager.io/issuer-kind": "Issuer", "cert-manager.io/issuer-group": "cert-manager.io"}
				require.NoError(t, kube.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret, b.store.namespace))
				certificate.Object["status"] = map[string]any{"conditions": []any{map[string]any{
					"type": "Ready", "status": "True", "observedGeneration": certificate.GetGeneration(),
				}}}
				_, err = b.store.dynamic.Resource(rotationCertificateResource).Namespace(b.store.namespace).Update(t.Context(), certificate, metav1.UpdateOptions{})
				require.NoError(t, err)
				return true, secret.DeepCopy(), nil
			})
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				require.ErrorIs(t, w.prepareTarget(t.Context(), target, nil, nil), errCertificateObserverUnavailable)
				w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
				require.NoError(t, w.prepareTarget(t.Context(), target, nil, nil))
				for _, plane := range w.state.Planes {
					require.Equal(t, plane.Previous.Source, plane.Source, "preparation has not changed trust")
					if plane.Affected {
						require.NotEqual(t, plane.OldCA, plane.NewCA)
						require.NotEmpty(t, plane.Signer.UID)
						require.Empty(t, plane.TargetIssuer.UID, "issuance is not switched during preparation")
					} else {
						require.Equal(t, plane.OldCA, plane.NewCA)
						require.Equal(t, plane.Previous.Issuer, plane.TargetIssuer)
					}
				}
				resumeExecutionFixture(t, w)
				version := w.journal.ResourceVersion
				require.NoError(t, w.prepareTarget(t.Context(), target, nil, nil))
				require.Equal(t, version, w.journal.ResourceVersion, "prepared retry performs no writes")
				require.NotContains(t, w.journal.Data[rotationJournalKey], "PRIVATE KEY")
				return nil
			}))
		})
	}
}
