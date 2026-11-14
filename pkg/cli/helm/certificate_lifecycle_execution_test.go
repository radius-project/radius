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
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// This fixture represents pre-staged public identities. It runs neither
// certificate controllers nor CSI and supplies no installed-product evidence.
func lifecycleExecutionFixture(t *testing.T) *certificateExecutionWriter {
	t.Helper()
	store, _ := bootstrapStoreFixture(t)
	return lifecycleExecutionFixtureInStore(t, store)
}

func lifecycleExecutionFixtureInStore(t *testing.T, store *rotationStore) *certificateExecutionWriter {
	t.Helper()
	state, _ := lifecycleFixture(t)
	state.Phase = certificatePrepare
	namespace, err := store.kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: store.namespace},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	state.Execution = &certificateLifecycleExecution{NamespaceUID: namespace.UID,
		ChartSHA256: strings.Repeat("a", 64), BaselineValuesSHA256: strings.Repeat("b", 64),
		TargetValuesSHA256: strings.Repeat("c", 64), BaselineVersion: 1,
		Resources: []certificateResourceIntent{{Kind: "Issuer", Name: "staged-root", Plane: "internal",
			Spec: map[string]any{"selfSigned": map[string]any{}}}},
	}
	for name, plane := range state.Planes {
		for _, previous := range []bool{true, false} {
			ref, pin := plane.Issuer, plane.Signer
			if previous {
				ref, pin = plane.Previous.Issuer.Reference, plane.Previous.Signer
			}
			ca, key, certificate := testSigningCA(t, nil, nil, nil, nil)
			secret := caTLSSecret(pin.Name, ca, key)
			secret.Namespace = state.Namespace
			secret, err := store.kube.CoreV1().Secrets(state.Namespace).Create(t.Context(), secret, metav1.CreateOptions{})
			require.NoError(t, err)
			pin.UID, pin.ResourceVersion = secret.UID, secret.ResourceVersion
			pin.SHA256 = fmt.Sprintf("%x", sha256.Sum256(certificate.Raw))
			_, err = store.dynamic.Resource(rotationIssuerResource).Namespace(state.Namespace).Create(t.Context(), &unstructured.Unstructured{
				Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
					"metadata": map[string]any{"name": ref.Name, "namespace": state.Namespace,
						"labels":      map[string]any{helmManagedByLabel: helmManagedByValue},
						"annotations": map[string]any{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace}},
					"spec": map[string]any{"ca": map[string]any{"secretName": pin.Name}}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			issuer, err := store.snapshotCertificateIssuer(t.Context(), ref)
			require.NoError(t, err)
			if previous {
				plane.OldCA, plane.Previous.Source.Bundle, plane.Previous.Issuer = ca, ca, issuer
				plane.Previous.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(ca))
			} else {
				plane.NewCA, plane.TargetIssuer = ca, issuer
			}
		}
		metadata := metav1.ObjectMeta{Name: plane.Source.Name, Namespace: state.Namespace,
			Labels: map[string]string{helmManagedByLabel: helmManagedByValue},
			Annotations: map[string]string{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace,
				"radius.io/certificate-mode": state.Mode, "helm.sh/resource-policy": "keep"}}
		var source metav1.Object
		if name == "internal" {
			source, err = store.kube.CoreV1().ConfigMaps(state.Namespace).Create(t.Context(), &corev1.ConfigMap{
				ObjectMeta: metadata, Data: map[string]string{caCertificateKey: string(plane.OldCA)},
			}, metav1.CreateOptions{})
		} else {
			metadata.Annotations[allowDirectInjectionAnnotation] = "true"
			source, err = store.kube.CoreV1().Secrets(state.Namespace).Create(t.Context(), &corev1.Secret{
				ObjectMeta: metadata, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{caCertificateKey: plane.OldCA},
			}, metav1.CreateOptions{})
		}
		require.NoError(t, err)
		plane.Previous.Source.UID, plane.Previous.Source.ResourceVersion = source.GetUID(), source.GetResourceVersion()
		plane.Source = plane.Previous.Source
		state.Planes[name] = plane
	}
	api, err := store.dynamic.Resource(apiServiceResource).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": radiusAPIService,
			"labels": map[string]any{helmManagedByLabel: helmManagedByValue},
			"annotations": map[string]any{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace,
				injectCAFromSecretAnnotation: ucpAPIServerCASource(state.Namespace)}},
		"spec": map[string]any{"group": "api.ucp.dev", "version": "v1alpha3",
			"groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": state.Namespace, "name": "ucp"},
			"caBundle": base64.StdEncoding.EncodeToString(state.Planes["aggregation"].OldCA)},
	}}, metav1.CreateOptions{})
	require.NoError(t, err)
	state.APIServiceUID = api.GetUID()
	require.NoError(t, state.validate(store.now()))
	return &certificateExecutionWriter{store: store, state: state}
}

func TestCertificateExecutionIssuerCASRecovery(t *testing.T) {
	for _, scenario := range []string{"success", "observation unavailable", "observation fails", "interrupted recording", "stale ready"} {
		t.Run(scenario, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			plane := w.state.Planes["internal"]
			plane.Issuer, plane.TargetIssuer = plane.Previous.Issuer.Reference, certificateIssuerSnapshot{}
			w.state.Planes["internal"] = plane
			w.state.Execution.Resources = []certificateResourceIntent{{Kind: "Issuer", Name: plane.Issuer.Name, Plane: "internal",
				Spec: map[string]any{"ca": map[string]any{"secretName": plane.Signer.Name}},
				Before: &aggregationBootstrapPin{UID: plane.Previous.Issuer.UID, ResourceVersion: plane.Previous.Issuer.ResourceVersion,
					SpecSHA256: plane.Previous.Issuer.SpecSHA256}}}
			// A controller fixture supplies status; this is not live issuance.
			stale := scenario == "stale ready"
			w.store.dynamic.(*dynamicfake.FakeDynamicClient).PrependReactor("update", "issuers", func(action k8stesting.Action) (bool, runtime.Object, error) {
				object := action.(k8stesting.UpdateAction).GetObject().(*unstructured.Unstructured)
				object.SetGeneration(2)
				generation := int64(2)
				if stale {
					generation = 1
				}
				object.Object["status"] = map[string]any{"conditions": []any{map[string]any{
					"type": "Ready", "status": "True", "observedGeneration": generation,
				}}}
				return false, nil, nil
			})
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				for _, name := range []string{"internal", "aggregation"} {
					require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/publish"))
				}
				convergeAPIServiceFixture(t, w)
				w.observeLoaded = func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
				for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue} {
					next, err := cloneCertificateLifecycle(w.state)
					require.NoError(t, err)
					next.Phase = phase
					require.NoError(t, w.save(t.Context(), next))
				}
				w.observeLoaded = nil
				calls := 0
				if scenario != "observation unavailable" {
					w.observeLoaded = func(ctx context.Context, state certificateLifecycle) error {
						// Explicitly injected orchestration fixture, not an
						// authenticated installed-product observation.
						calls++
						require.Equal(t, rotationIssue, state.Phase)
						if scenario == "observation fails" {
							return errors.New("fixture: consumer has not loaded overlap")
						}
						return ctx.Err()
					}
				}
				if scenario == "interrupted recording" {
					fail := true
					w.store.kube.(*kubefake.Clientset).PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
						object := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
						if !fail || object.Name != rotationJournalName {
							return false, nil, nil
						}
						state, err := decodeCertificateLifecycle(object, w.store.namespace, w.store.release, w.store.now())
						require.NoError(t, err)
						if state.Execution.Resources[0].Observed == nil {
							return false, nil, nil
						}
						fail = false
						return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, object.Name, errors.New("fixture interruption"))
					})
				}
				ctx := t.Context()
				if stale {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 20*time.Millisecond)
					defer cancel()
				}
				err := w.applyIssuer(ctx, 0)
				switch scenario {
				case "observation unavailable":
					require.ErrorIs(t, err, errCertificateObserverUnavailable)
				case "observation fails":
					require.ErrorContains(t, err, "has not loaded")
				case "interrupted recording":
					require.ErrorContains(t, err, "fixture interruption")
					resumeExecutionFixture(t, w)
					require.NoError(t, w.applyIssuer(t.Context(), 0))
				case "stale ready":
					require.ErrorIs(t, err, context.DeadlineExceeded)
					require.Nil(t, w.state.Execution.Resources[0].Observed)
					require.NotEmpty(t, w.state.Execution.PendingResource)
					stale = false
					object, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Get(t.Context(), plane.Issuer.Name, metav1.GetOptions{})
					require.NoError(t, err)
					_, err = w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Update(t.Context(), object, metav1.UpdateOptions{})
					require.NoError(t, err)
					resumeExecutionFixture(t, w)
					require.NoError(t, w.applyIssuer(t.Context(), 0))
				default:
					require.NoError(t, err)
					require.Equal(t, 2, calls)
				}
				observed, err := w.store.snapshotCertificateIssuer(t.Context(), plane.Issuer)
				require.NoError(t, err)
				if strings.HasPrefix(scenario, "observation") {
					require.Equal(t, plane.Previous.Issuer, observed, "missing proof never mutates issuance")
				} else {
					require.Equal(t, plane.Signer.Name, observed.SigningSecret)
					require.Equal(t, plane.Previous.Issuer.UID, observed.UID)
					require.NoError(t, w.applyIssuer(t.Context(), 0))
				}
				return nil
			}))
		})
	}
}

// This models cainjector output only, not the aggregator loading that output.
func convergeAPIServiceFixture(t *testing.T, w *certificateExecutionWriter) {
	t.Helper()
	api, err := w.store.dynamic.Resource(apiServiceResource).Get(t.Context(), radiusAPIService, metav1.GetOptions{})
	require.NoError(t, err)
	_, bundle, err := w.readSource(t.Context(), "aggregation")
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(api.Object, base64.StdEncoding.EncodeToString(bundle), "spec", "caBundle"))
	_, err = w.store.dynamic.Resource(apiServiceResource).Update(t.Context(), api, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func TestCertificateExecutionRetirementRecovery(t *testing.T) {
	for _, interruptedPlane := range []string{"internal", "aggregation", "missing proof"} {
		t.Run(interruptedPlane, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			// Injected observation fixtures test ordering, never authentication.
			observe := func(ctx context.Context, _ certificateLifecycle) error { return ctx.Err() }
			w.observeLoaded, w.observeServing = observe, observe
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				require.NoError(t, w.ensureResource(t.Context(), 0))
				for _, name := range []string{"internal", "aggregation"} {
					require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/publish"))
				}
				convergeAPIServiceFixture(t, w)
				for _, phase := range []rotationPhase{rotationPublish, rotationAdopt, rotationIssue, rotationReady, rotationRetire} {
					next, err := cloneCertificateLifecycle(w.state)
					require.NoError(t, err)
					next.Phase = phase
					require.NoError(t, w.save(t.Context(), next))
				}
				if interruptedPlane == "missing proof" {
					before := w.journal.ResourceVersion
					w.observeLoaded = nil
					require.ErrorIs(t, w.publishSource(t.Context(), "internal", w.state.ID+"/complete"), errCertificateObserverUnavailable)
					require.Equal(t, before, w.journal.ResourceVersion)
					w.observeLoaded = observe
				} else {
					fail := true
					w.store.kube.(*kubefake.Clientset).PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
						object := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
						if !fail || object.Name != rotationJournalName {
							return false, nil, nil
						}
						next, err := decodeCertificateLifecycle(object, w.store.namespace, w.store.release, w.store.now())
						require.NoError(t, err)
						if next.Execution.PendingSource != nil || next.Planes[interruptedPlane].Source.Revision != w.state.ID+"/complete" {
							return false, nil, nil
						}
						fail = false
						return true, nil, errors.New("fixture interruption after trust removal")
					})
				}
				for _, name := range []string{"internal", "aggregation"} {
					err := w.publishSource(t.Context(), name, w.state.ID+"/complete")
					if name == interruptedPlane {
						require.ErrorContains(t, err, "fixture interruption")
						resumeExecutionFixture(t, w)
						require.NoError(t, w.publishSource(t.Context(), name, w.state.ID+"/complete"))
					} else {
						require.NoError(t, err)
					}
					require.Equal(t, w.state.Planes[name].NewCA, w.state.Planes[name].Source.Bundle)
				}
				next, err := cloneCertificateLifecycle(w.state)
				require.NoError(t, err)
				next.Phase = rotationComplete
				require.ErrorContains(t, w.save(t.Context(), next), "APIService has not converged")
				convergeAPIServiceFixture(t, w)
				require.NoError(t, w.save(t.Context(), next))
				return nil
			}))
		})
	}
}

func resumeExecutionFixture(t *testing.T, w *certificateExecutionWriter) {
	t.Helper()
	journal, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(t.Context(), rotationJournalName, metav1.GetOptions{})
	require.NoError(t, err)
	state, err := decodeCertificateLifecycle(journal, w.store.namespace, w.store.release, w.store.now())
	require.NoError(t, err)
	w.state, w.journal = state, journal
}

func TestCertificateExecutionIntentRecovery(t *testing.T) {
	for _, step := range []string{"resource", "internal", "aggregation"} {
		t.Run(step, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			kube := w.store.kube.(*kubefake.Clientset)
			require.NoError(t, withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				fail := true
				kube.PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
					object := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
					if !fail || object.Name != rotationJournalName {
						return false, nil, nil
					}
					next, err := decodeCertificateLifecycle(object, w.store.namespace, w.store.release, w.store.now())
					require.NoError(t, err)
					recording := step == "resource" && next.Execution.Resources[0].Observed != nil ||
						step != "resource" && next.Execution.PendingSource == nil && next.Planes[step].Source.JournalUID != ""
					if !recording {
						return false, nil, nil
					}
					fail = false
					return true, nil, apierrors.NewConflict(schema.GroupResource{Resource: "configmaps"}, object.Name, errors.New("fixture interruption"))
				})
				run := func() error {
					if step == "resource" {
						return w.ensureResource(t.Context(), 0)
					}
					return w.publishSource(t.Context(), step, w.state.ID+"/publish")
				}
				require.ErrorContains(t, run(), "fixture interruption")
				resumeExecutionFixture(t, w)
				require.NoError(t, run(), "recover the exact owned creation/write, not a replacement")
				after := w.journal.ResourceVersion
				require.NoError(t, run())
				require.Equal(t, after, w.journal.ResourceVersion, "completed bounded writes are idempotent")
				require.NotContains(t, w.journal.Data[rotationJournalKey], "PRIVATE KEY")
				return nil
			}))
		})
	}
}

func TestCertificateExecutionPreWriteFences(t *testing.T) {
	for _, scenario := range []string{"changed signer", "changed source", "changed issuer", "changed journal", "lost lease", "canceled", "pending bootstrap", "foreign resource"} {
		t.Run(scenario, func(t *testing.T) {
			w := lifecycleExecutionFixture(t)
			err := withIdentityOperation(t.Context(), w.store.kube, w.store.namespace, func(operation *identityOperation) error {
				w.operation = operation
				require.NoError(t, w.save(t.Context(), w.state))
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch scenario {
				case "changed signer":
					secret, err := w.store.kube.CoreV1().Secrets(w.store.namespace).Get(ctx, w.state.Planes["internal"].Signer.Name, metav1.GetOptions{})
					require.NoError(t, err)
					secret.Data[corev1.TLSPrivateKeyKey] = []byte("not-a-key")
					_, err = w.store.kube.CoreV1().Secrets(w.store.namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)
				case "changed source":
					source, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Get(ctx, "radius-mtls-ca", metav1.GetOptions{})
					require.NoError(t, err)
					source.Data[caCertificateKey] = string(w.state.Planes["internal"].NewCA)
					_, err = w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(ctx, source, metav1.UpdateOptions{})
					require.NoError(t, err)
				case "changed issuer":
					issuer, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Get(ctx, w.state.Planes["internal"].Issuer.Name, metav1.GetOptions{})
					require.NoError(t, err)
					issuer.Object["spec"] = map[string]any{"ca": map[string]any{"secretName": "foreign"}}
					_, err = w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Update(ctx, issuer, metav1.UpdateOptions{})
					require.NoError(t, err)
				case "changed journal":
					_, err := w.store.kube.CoreV1().ConfigMaps(w.store.namespace).Update(ctx, w.journal.DeepCopy(), metav1.UpdateOptions{})
					require.NoError(t, err)
				case "lost lease":
					require.NoError(t, w.store.kube.CoordinationV1().Leases(w.store.namespace).Delete(ctx, identityOperationLeaseName, metav1.DeleteOptions{}))
				case "canceled":
					cancel()
				case "pending bootstrap":
					namespace, err := w.store.kube.CoreV1().Namespaces().Get(ctx, w.store.namespace, metav1.GetOptions{})
					require.NoError(t, err)
					namespace.Annotations = map[string]string{initialCertificateIntent: "pending"}
					_, err = w.store.kube.CoreV1().Namespaces().Update(ctx, namespace, metav1.UpdateOptions{})
					require.NoError(t, err)
				case "foreign resource":
					_, err := w.store.dynamic.Resource(rotationIssuerResource).Namespace(w.store.namespace).Create(ctx, &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "cert-manager.io/v1", "kind": "Issuer", "metadata": map[string]any{"name": "staged-root", "namespace": w.store.namespace},
						"spec": map[string]any{"selfSigned": map[string]any{}},
					}}, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				require.Error(t, w.ensureResource(ctx, 0))
				return nil
			})
			if scenario == "lost lease" {
				require.ErrorContains(t, err, "cannot release")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
