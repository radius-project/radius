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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestIssuerSnapshotRevisionAndReplacement(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"Issuer", "ClusterIssuer"} {
		for _, scenario := range []string{"unchanged", "status update", "replacement", "changed spec", "terminating", "canceled"} {
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				namespace, resource := RadiusSystemNamespace, rotationIssuerResource
				if kind == "ClusterIssuer" {
					namespace, resource.Resource = "", "clusterissuers"
				}
				issuer := &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "cert-manager.io/v1", "kind": kind,
					"metadata": map[string]any{"name": "selected", "namespace": namespace, "uid": "issuer", "resourceVersion": "1"},
					"spec":     map[string]any{"vault": map[string]any{"server": "https://issuer.example"}},
				}}
				client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), issuer)
				store := &rotationStore{dynamic: client, namespace: RadiusSystemNamespace}
				ref := certificateIssuerReference{Group: "cert-manager.io", Kind: kind, Name: "selected"}
				pin, err := store.snapshotCertificateIssuer(t.Context(), ref)
				require.NoError(t, err)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				changed := issuer.DeepCopy()
				switch scenario {
				case "status update":
					changed.SetResourceVersion("2")
					changed.Object["status"] = map[string]any{"ready": true}
				case "replacement":
					changed.SetUID("replacement")
				case "changed spec":
					changed.Object["spec"] = map[string]any{"vault": map[string]any{"server": "https://changed.example"}}
				case "terminating":
					now := metav1.Now()
					changed.SetDeletionTimestamp(&now)
				case "canceled":
					cancel()
				}
				require.NoError(t, client.Tracker().Update(resource, changed, namespace))
				for _, phase := range []rotationPhase{rotationPublish, rotationComplete} {
					err := store.checkCertificateIssuerSnapshot(ctx, pin, phase)
					if scenario == "unchanged" || scenario == "status update" && phase == rotationComplete {
						require.NoError(t, err)
					} else {
						require.Error(t, err)
					}
				}
				encoded, err := json.Marshal(pin)
				require.NoError(t, err)
				require.NotContains(t, string(encoded), "https://issuer.example", "arbitrary operator specs must not be copied into public journals")
				for _, action := range client.Actions() {
					require.Equal(t, "get", action.GetVerb())
				}
			})
		}
	}
}

func TestCertificatePlaneSnapshotPublicMaterialOnly(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			state, _ := lifecycleFixture(t)
			plane := state.Planes["aggregation"]
			source := permanentPublicTrustSecret(plane.OldCA)
			source.Annotations["radius.io/certificate-mode"] = mode
			signer := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "signer", Namespace: state.Namespace,
				UID: "signer-uid", ResourceVersion: "1"}, Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{corev1.TLSCertKey: plane.OldCA, corev1.TLSPrivateKeyKey: []byte("never-journal-this-private-key")}}
			kube := kubefake.NewClientset(source, signer)
			issuer := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
				"metadata": map[string]any{"name": "issuer", "namespace": state.Namespace, "uid": "issuer", "resourceVersion": "1"},
				"spec":     map[string]any{"ca": map[string]any{"secretName": "signer"}},
			}}
			store := &rotationStore{kube: kube, dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), issuer),
				namespace: state.Namespace, release: state.Release}
			snapshot, err := store.snapshotCertificatePlane(t.Context(), "aggregation", mode,
				certificateIssuerReference{Group: "cert-manager.io", Kind: "Issuer", Name: "issuer"})
			require.NoError(t, err)
			require.Equal(t, source.UID, snapshot.Source.UID)
			require.Equal(t, plane.OldCA, snapshot.Source.Bundle)
			data, err := json.Marshal(snapshot)
			require.NoError(t, err)
			require.NotContains(t, string(data), "never-journal-this-private-key")
			require.NotContains(t, string(data), "tls.key")
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
			if mode == "issuerRef" {
				require.Nil(t, snapshot.Signer)
				require.Len(t, kube.Actions(), 1, "external issuer snapshot must not read the signing Secret")
			} else {
				require.Equal(t, signer.UID, snapshot.Signer.UID)
				require.Equal(t, plane.Previous.Signer.SHA256, snapshot.Signer.SHA256)
			}
		})
	}
}

func TestLifecycleRejectsUnstageableIssuerAndChangedHistory(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"same-name external issuer", "missing history", "changed historical source", "different target reference", "wrong signer selection"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			state, _ := lifecycleFixture(t)
			plane := state.Planes["aggregation"]
			switch scenario {
			case "same-name external issuer":
				state.Mode = "issuerRef"
				for name, current := range state.Planes {
					current.Signer = nil
					state.Planes[name] = current
				}
				plane.Signer = nil
				plane.Issuer = plane.Previous.Issuer.Reference
				plane.TargetIssuer.Reference = plane.Issuer
			case "missing history":
				plane.Previous = nil
			case "changed historical source":
				plane.Previous.Source.Bundle = plane.NewCA
			case "different target reference":
				plane.TargetIssuer.Reference.Name = "unselected"
			case "wrong signer selection":
				plane.TargetIssuer.SigningSecret = "another-signer"
			}
			state.Planes["aggregation"] = plane
			err := state.validate(time.Now())
			require.Error(t, err)
			if strings.HasPrefix(scenario, "same-name") {
				require.ErrorContains(t, err, "distinct staged issuer")
			}
		})
	}
}

func TestLifecycleRecoveryUsesDurablePublicAndIssuerPins(t *testing.T) {
	t.Parallel()
	for _, phase := range []rotationPhase{rotationPublish, rotationComplete} {
		for _, scenario := range []string{"unchanged", "signer metadata", "signer replacement", "issuer configuration", "journal revision"} {
			t.Run(string(phase)+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				state, journal := lifecycleFixture(t)
				state.Phase = phase
				var coreObjects, dynamicObjects []runtime.Object
				for name, plane := range state.Planes {
					old, oldKey, oldCertificate := testSigningCA(t, nil, nil, nil, nil)
					plane.OldCA, plane.Previous.Source.Bundle = old, old
					plane.Previous.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(old))
					plane.Previous.Signer.SHA256 = fmt.Sprintf("%x", sha256.Sum256(oldCertificate.Raw))
					coreObjects = append(coreObjects, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
						Name: plane.Previous.Signer.Name, Namespace: state.Namespace,
						UID: plane.Previous.Signer.UID, ResourceVersion: plane.Previous.Signer.ResourceVersion},
						Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: old, corev1.TLSPrivateKeyKey: oldKey}})
					next, key, certificate := testSigningCA(t, nil, nil, nil, nil)
					plane.NewCA = next
					plane.Signer.SHA256 = fmt.Sprintf("%x", sha256.Sum256(certificate.Raw))
					plane.Source.Bundle = next
					if phase == rotationPublish {
						plane.Source.Bundle = append(append([]byte{}, plane.OldCA...), next...)
					}
					plane.Source.SHA256 = fmt.Sprintf("%x", sha256.Sum256(plane.Source.Bundle))
					secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: plane.Signer.Name,
						Namespace: state.Namespace, UID: plane.Signer.UID, ResourceVersion: plane.Signer.ResourceVersion},
						Type: corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: next, corev1.TLSPrivateKeyKey: key}}
					if name == "aggregation" {
						switch scenario {
						case "signer metadata":
							secret.ResourceVersion = "later"
						case "signer replacement":
							secret.UID = "replacement"
						}
					}
					coreObjects = append(coreObjects, secret)
					for _, pin := range []*certificateIssuerSnapshot{&plane.Previous.Issuer, &plane.TargetIssuer} {
						issuer := &unstructured.Unstructured{Object: map[string]any{
							"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
							"metadata": map[string]any{"name": pin.Reference.Name, "namespace": state.Namespace,
								"uid": string(pin.UID), "resourceVersion": pin.ResourceVersion},
							"spec": map[string]any{"ca": map[string]any{"secretName": pin.SigningSecret}},
						}}
						var err error
						pin.SpecSHA256, err = rotationIssuerSpecDigest(issuer)
						require.NoError(t, err)
						if name == "aggregation" && pin == &plane.TargetIssuer && scenario == "issuer configuration" {
							issuer.Object["spec"] = map[string]any{"ca": map[string]any{"secretName": "unselected"}}
						}
						dynamicObjects = append(dynamicObjects, issuer)
					}
					metadata := metav1.ObjectMeta{Name: plane.Source.Name, Namespace: state.Namespace, UID: plane.Source.UID,
						ResourceVersion: plane.Source.ResourceVersion, Labels: map[string]string{helmManagedByLabel: helmManagedByValue},
						Annotations: map[string]string{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace,
							rotationJournalUID: string(journal.UID), rotationRevision: state.ID + "/" + string(phase)}}
					if name == "internal" {
						coreObjects = append(coreObjects, &corev1.ConfigMap{ObjectMeta: metadata, Data: map[string]string{caCertificateKey: string(plane.Source.Bundle)}})
					} else {
						metadata.Annotations[allowDirectInjectionAnnotation] = "true"
						coreObjects = append(coreObjects, &corev1.Secret{ObjectMeta: metadata, Type: corev1.SecretTypeOpaque,
							Data: map[string][]byte{caCertificateKey: plane.Source.Bundle}})
					}
					state.Planes[name] = plane
				}
				dynamicObjects = append(dynamicObjects, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
					"metadata": map[string]any{"name": radiusAPIService, "uid": string(state.APIServiceUID),
						"labels":      map[string]any{helmManagedByLabel: helmManagedByValue},
						"annotations": map[string]any{helmReleaseNameAnnotation: state.Release, helmReleaseNamespaceAnnotation: state.Namespace}},
				}})
				data, err := json.Marshal(state)
				require.NoError(t, err)
				journal.Data = map[string]string{rotationJournalKey: string(data)}
				live := journal.DeepCopy()
				if scenario == "journal revision" {
					live.ResourceVersion = "replaced"
				}
				coreObjects = append(coreObjects, live)
				kube := kubefake.NewClientset(coreObjects...)
				client := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme(), dynamicObjects...)
				store := &rotationStore{kube: kube, dynamic: client, namespace: state.Namespace, release: state.Release, now: time.Now}
				err = store.checkCertificateLifecycleRecovery(t.Context(), journal)
				if scenario == "unchanged" || scenario == "signer metadata" && phase == rotationComplete {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				for _, action := range append(kube.Actions(), client.Actions()...) {
					require.Equal(t, "get", action.GetVerb(), "recovery inspection is not execution or a workload-readiness claim")
				}
			})
		}
	}
}
