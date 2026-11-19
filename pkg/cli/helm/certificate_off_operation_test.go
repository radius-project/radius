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

	"github.com/radius-project/radius/pkg/authz"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestCertificateOffOperationLeaseAndMutationGuards(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	for _, scenario := range []string{"success", "cancel", "lease changed", "journal appeared", "serving observation failed", "APIService replaced"} {
		t.Run(scenario, func(t *testing.T) {
			store := bootstrapStoreFixture(t)
			_, err := store.kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: store.namespace},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			old, next := newTestCA(t, "old"), newTestCA(t, "off")
			api := permanentTrustAPIService(old.pem)
			_, err = store.dynamic.Resource(apiServicesResource).Create(t.Context(), api, metav1.CreateOptions{})
			require.NoError(t, err)
			replaced := false
			impl := &Impl{
				newRotationStore: func(string, string) (*rotationStore, error) { return store, nil },
				observeUCPOffServing: func(ctx context.Context, _, _ string, _ kubernetes.Interface) error {
					if scenario == "serving observation failed" {
						return errors.New("exact-Pod observation failed")
					}
					if scenario == "APIService replaced" && !replaced {
						replaced = true
						client := store.dynamic.Resource(apiServicesResource)
						require.NoError(t, client.Delete(ctx, api.GetName(), metav1.DeleteOptions{}))
						replacement := api.DeepCopy()
						replacement.SetUID("replacement")
						replacement.SetResourceVersion("")
						_, err := client.Create(ctx, replacement, metav1.CreateOptions{})
						require.NoError(t, err)
					}
					return nil // Fixture acknowledgement, not live serving evidence.
				},
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			err = impl.withCertificateOffOperation(ctx, "", store.namespace, authz.ModeOff, func(ctx context.Context) error {
				secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ucpOffCertificateSecretName},
					Data: map[string][]byte{caCertificateKey: next.pem}}
				if err := applyHelmOwnedSecret(ctx, "", store.kube, secret, store.release, store.namespace); err != nil {
					return err
				}
				switch scenario {
				case "cancel":
					cancel()
				case "lease changed":
					client := store.kube.CoordinationV1().Leases(store.namespace)
					lease, err := client.Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
					require.NoError(t, err)
					_, err = client.Update(ctx, lease, metav1.UpdateOptions{})
					require.NoError(t, err)
				case "journal appeared":
					_, err := store.kube.CoreV1().ConfigMaps(store.namespace).Create(ctx, &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{Name: rotationJournalName},
					}, metav1.CreateOptions{})
					require.NoError(t, err)
				}
				ctx.Value(certificateOffOperationKey{}).(*certificateOffOperation).postHelm = true
				return impl.withCertificateOffOperation(ctx, "", store.namespace, authz.ModeOff, func(ctx context.Context) error {
					return settleUCPOffTrust(ctx, "", store.dynamic, testCACertificates(t, next.pem))
				})
			})
			live, readErr := store.dynamic.Resource(apiServicesResource).Get(t.Context(), api.GetName(), metav1.GetOptions{})
			require.NoError(t, readErr)
			if scenario == "success" {
				require.NoError(t, err)
				clean, err := ucpOffTrustClean(live, testCACertificates(t, next.pem))
				require.NoError(t, err)
				require.True(t, clean)
			} else {
				require.Error(t, err)
				require.Equal(t, api.Object["spec"], live.Object["spec"])
				require.Equal(t, api.GetAnnotations(), live.GetAnnotations())
			}
			creates := 0
			for _, action := range store.kube.(*kubefake.Clientset).Actions() {
				if action.GetVerb() == "create" && action.GetResource().Resource == "leases" {
					creates++
				}
			}
			require.Equal(t, 1, creates, "nested post-Helm work must share the operation Lease")
		})
	}
}

func TestUCPMutationGuardErrorsAreNotRetried(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	for _, operation := range []string{"pre-Helm APIService", "post-Helm APIService", "on APIService",
		"Secret create", "Secret update", "public source create", "public source update", "shared source delete"} {
		t.Run(operation, func(t *testing.T) {
			store := bootstrapStoreFixture(t)
			old, next := newTestCA(t, "old"), newTestCA(t, "next")
			api := permanentTrustAPIService(old.pem)
			_, err := store.dynamic.Resource(apiServicesResource).Create(t.Context(), api, metav1.CreateOptions{})
			require.NoError(t, err)
			desired := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: ucpOffCertificateSecretName},
				Data: map[string][]byte{caCertificateKey: next.pem}}
			if operation == "Secret update" {
				initial := desired.DeepCopy()
				initial.Data = map[string][]byte{caCertificateKey: old.pem}
				require.NoError(t, applyHelmOwnedSecret(t.Context(), "", store.kube, initial, store.release, store.namespace))
			}
			if operation == "public source update" || operation == "shared source delete" {
				_, err := writeUCPAPIServerTrust(t.Context(), store.kube, store.namespace, testCACertificates(t, old.pem))
				require.NoError(t, err)
			}
			calls := 0
			blocked := apierrors.NewConflict(schema.GroupResource{Resource: "guard"}, "guard", errors.New("stop"))
			ctx := withUCPMutationGuard(t.Context(), func(context.Context, string, metav1.PartialObjectMetadata) (bool, error) {
				calls++
				return false, blocked
			})
			switch operation {
			case "pre-Helm APIService":
				err = trustUCPAPIServerCAs(ctx, "", store.dynamic, testCACertificates(t, next.pem))
			case "post-Helm APIService":
				err = settleUCPOffTrust(ctx, "", store.dynamic, testCACertificates(t, next.pem))
			case "on APIService":
				err = injectUCPAPIServerCA(ctx, store.dynamic, store.namespace, next.pem)
			case "public source create", "public source update":
				_, err = writeUCPAPIServerTrust(ctx, store.kube, store.namespace, testCACertificates(t, next.pem))
			case "shared source delete":
				err = (&Impl{}).deleteOwnedUCPAPIServerTrust(ctx, "", store.kube, store.namespace)
			default:
				err = applyHelmOwnedSecret(ctx, "", store.kube, desired, store.release, store.namespace)
			}
			require.ErrorIs(t, err, blocked)
			require.Equal(t, 1, calls, "a guard conflict is not an optimistic-write retry")
			live, err := store.dynamic.Resource(apiServicesResource).Get(t.Context(), api.GetName(), metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, api.Object["spec"], live.Object["spec"])
		})
	}
}

func TestCertificateOffPublicEntryPointCannotBypassJournals(t *testing.T) {
	for _, name := range []string{initialCertificateJournal, rotationJournalName, identityProviderJournal} {
		t.Run(name, func(t *testing.T) {
			cluster := newOffAfterOnCluster(t)
			impl := cluster.impl(nil)
			_, err := cluster.kube.CoreV1().ConfigMaps(RadiusSystemNamespace).Create(t.Context(), &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Data:       map[string]string{"phase": "complete"},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
			before := len(cluster.clusterWrites())
			require.ErrorContains(t, impl.waitForUCPAPIService(t.Context(), "", RadiusSystemNamespace, authz.ModeOff, nil), name)
			require.Empty(t, cluster.clusterWrites()[before:], "even completed intent is rejected before Lease or trust writes")
			require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, cluster.dynamic))
		})
	}
}

func TestCertificateOffPublicEntryPointComposesRetention(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	cluster := newOffAfterOnCluster(t)
	cluster.putTrustSecret(t, true)
	impl := cluster.impl(nil)
	calls := 0
	impl.ucpMutationGuard = func(_ context.Context, verb string, object metav1.PartialObjectMetadata) (bool, error) {
		calls++
		return verb == "delete" && object.Name == ucpAPIServerTrustSecretName, nil
	}
	require.NoError(t, impl.waitForUCPAPIService(t.Context(), "", RadiusSystemNamespace, authz.ModeOff, manifestObjectSet{manifestObjectKey("", "Secret", ucpOffCertificateSecretName): true}))
	require.Positive(t, calls)
	_, err := cluster.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err, "retention does not bypass the operation guard")
	for _, name := range []string{ucpAPIServerCASecretName, ucpAPIServerTLSSecretName} {
		_, err := cluster.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err))
	}
	require.Equal(t, []string{"ucp-ca"}, apiServiceCABundle(t, cluster.dynamic))
}

func TestCertificateOffPublicEntryPointPinsServingSource(t *testing.T) {
	shortenUCPAPIServiceWait(t)
	cluster := newOffAfterOnCluster(t)
	impl := cluster.impl(nil)
	impl.observeUCPOffServing = func(ctx context.Context, _, _ string, _ kubernetes.Interface) error {
		secrets := cluster.kube.CoreV1().Secrets(RadiusSystemNamespace)
		source, err := secrets.Get(ctx, ucpOffCertificateSecretName, metav1.GetOptions{})
		require.NoError(t, err)
		_, err = secrets.Update(ctx, source, metav1.UpdateOptions{})
		require.NoError(t, err)
		return nil
	}
	require.ErrorContains(t, impl.waitForUCPAPIService(t.Context(), "", RadiusSystemNamespace, authz.ModeOff, manifestObjectSet{manifestObjectKey("", "Secret", ucpOffCertificateSecretName): true}), "Off serving source changed")
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, cluster.dynamic))
	_, err := cluster.kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), ucpAPIServerCASecretName, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestCertificateOffPublicEntryPointRequiresPostHelmScope(t *testing.T) {
	cluster := newOffAfterOnCluster(t)
	impl := cluster.impl(nil)
	require.ErrorContains(t, impl.ReconcileUCPOffAPIServerTrust(t.Context(), "", RadiusSystemNamespace, nil), "post-Helm operation scope")
	require.ErrorContains(t, impl.withCertificateOffOperation(t.Context(), "", RadiusSystemNamespace, authz.ModeOff,
		func(ctx context.Context) error {
			return impl.ReconcileUCPOffAPIServerTrust(ctx, "", RadiusSystemNamespace, nil)
		}), "post-Helm operation scope")
	require.Equal(t, []string{"ucp-ca", "radius-apiserver-ca"}, apiServiceCABundle(t, cluster.dynamic))
}
