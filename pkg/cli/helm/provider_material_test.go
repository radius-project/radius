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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestProviderManagedCredentialsUseUnchangedSelectedCA(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, scenario := range []string{"create", "retry", "read only", "foreign", "broader identity", "orphan Secret", "interrupted"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				ca := rotationState{ID: uuid.NewString(), Namespace: RadiusSystemNamespace, Release: radiusReleaseName, Phase: rotationComplete,
					Plan: &rotationPlan{Issuer: "selected-ca", TargetMode: mode}}
				if mode == "issuerRef" {
					ca.Plan.ExternalIssuer = &rotationExternalIssuer{Group: "cert-manager.io", Kind: "ClusterIssuer", Name: "selected-ca"}
				}
				dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
				dyn.PrependReactor("create", "certificates", func(action ktesting.Action) (bool, runtime.Object, error) {
					object := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured)
					object.SetUID(types.UID(uuid.NewString()))
					object.SetResourceVersion("1")
					return false, nil, nil
				})
				store := &rotationStore{kube: fake.NewClientset(), dynamic: dyn, namespace: ca.Namespace, release: ca.Release, now: time.Now}
				client := dyn.Resource(rotationCertificateResource).Namespace(ca.Namespace)
				if scenario == "foreign" || scenario == "broader identity" {
					object := &unstructured.Unstructured{Object: map[string]any{
						"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
						"metadata": map[string]any{"name": "bicep-de-cert", "namespace": ca.Namespace,
							"annotations": map[string]any{"meta.helm.sh/release-name": ca.Release, "meta.helm.sh/release-namespace": ca.Namespace}},
						"spec": providerComponentCertificateSpec(ca, ca.Namespace, "bicep-de"),
					}}
					if scenario == "foreign" {
						object.SetAnnotations(nil)
					} else {
						require.NoError(t, unstructured.SetNestedField(object.Object, true, "spec", "isCA"))
					}
					_, err := client.Create(t.Context(), object, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				if scenario == "orphan Secret" {
					_, err := store.kube.CoreV1().Secrets(ca.Namespace).Create(t.Context(), &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{Name: "bicep-de-mtls", Namespace: ca.Namespace},
					}, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				dyn.ClearActions()
				writes := 0
				guard := func(context.Context) error {
					writes++
					if scenario == "interrupted" && writes == 3 {
						return errors.New("shared lease lost")
					}
					return nil
				}
				err := store.prepareProviderCertificates(t.Context(), ca, guard, scenario == "read only")
				switch scenario {
				case "foreign", "broader identity", "orphan Secret":
					require.Error(t, err)
					require.Zero(t, writes, "precheck all existing resources before creating any Certificate")
				case "interrupted":
					require.ErrorContains(t, err, "shared lease")
					require.NoError(t, store.prepareProviderCertificates(t.Context(), ca, guard, false))
				default:
					require.NoError(t, err)
				}
				if scenario == "retry" {
					before := writes
					require.NoError(t, store.prepareProviderCertificates(t.Context(), ca, guard, false))
					require.Equal(t, before, writes, "exact retry must not rewrite a Certificate")
				}
				if scenario == "read only" {
					require.Zero(t, writes)
				}
				for _, action := range dyn.Actions() {
					if action.GetVerb() == "create" || action.GetVerb() == "update" {
						require.Equal(t, "certificates", action.GetResource().Resource)
						var object *unstructured.Unstructured
						if create, ok := action.(ktesting.CreateAction); ok {
							object = create.GetObject().(*unstructured.Unstructured)
						} else {
							object = action.(ktesting.UpdateAction).GetObject().(*unstructured.Unstructured)
						}
						require.NotEqual(t, "ucp-cert-aggregation", object.GetName())
						issuer, _, err := unstructured.NestedMap(object.Object, "spec", "issuerRef")
						require.NoError(t, err)
						require.Equal(t, ca.Plan.issuerReference(), issuer)
					}
				}
				for _, action := range store.kube.(*fake.Clientset).Actions() {
					if scenario != "orphan Secret" {
						require.Equal(t, "get", action.GetVerb(), "preparation must never write private keys or signing CA material")
					}
				}
			})
		}
	}
}

func TestProviderRetiresOnlyPinnedFormerComponentSecrets(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"complete", "retry", "replacement", "certificate remains", "guard denied"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			_, plan, _, kube := providerRecordingFixture(t, "radius", "external")
			dyn := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
			store := &rotationStore{kube: kube, dynamic: dyn, namespace: plan.Namespace, release: plan.Release}
			for _, component := range rotationComponents {
				secret, err := kube.CoreV1().Secrets(plan.Namespace).Create(t.Context(), &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: component + "-mtls", Namespace: plan.Namespace},
				}, metav1.CreateOptions{})
				require.NoError(t, err)
				for index := range plan.SourceSecrets {
					if plan.SourceSecrets[index].Name == secret.Name {
						plan.SourceSecrets[index].UID = secret.UID
					}
				}
			}
			root, err := kube.CoreV1().Secrets(plan.Namespace).Create(t.Context(), &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "radius-ca", Namespace: plan.Namespace},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			if scenario == "replacement" {
				secret, err := kube.CoreV1().Secrets(plan.Namespace).Get(t.Context(), plan.SourceSecrets[0].Name, metav1.GetOptions{})
				require.NoError(t, err)
				secret.UID = types.UID(uuid.NewString())
				_, err = kube.CoreV1().Secrets(plan.Namespace).Update(t.Context(), secret, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			if scenario == "certificate remains" {
				_, err := dyn.Resource(rotationCertificateResource).Namespace(plan.Namespace).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
					"metadata": map[string]any{"name": "ucp-cert", "namespace": plan.Namespace},
				}}, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			kube.ClearActions()
			guard := func(context.Context) error {
				if scenario == "guard denied" {
					return errors.New("lease replaced")
				}
				return nil
			}
			err = store.retireProviderSecrets(t.Context(), plan, guard)
			if scenario == "complete" || scenario == "retry" {
				require.NoError(t, err)
				require.NoError(t, store.retireProviderSecrets(t.Context(), plan, guard))
				remaining, err := kube.CoreV1().Secrets(plan.Namespace).List(t.Context(), metav1.ListOptions{})
				require.NoError(t, err)
				require.Len(t, remaining.Items, 1)
				require.Equal(t, root.UID, remaining.Items[0].UID)
			} else {
				require.Error(t, err)
			}
			for _, action := range kube.Actions() {
				if deleted, ok := action.(ktesting.DeleteAction); ok {
					require.Equal(t, "secrets", action.GetResource().Resource)
					require.NotEqual(t, root.Name, deleted.GetName())
					require.NotEmpty(t, *deleted.GetDeleteOptions().Preconditions.UID)
					require.NotEmpty(t, *deleted.GetDeleteOptions().Preconditions.ResourceVersion)
				}
			}
		})
	}
}
