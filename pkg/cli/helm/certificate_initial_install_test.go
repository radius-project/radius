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
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

// initialControllerFixture simulates issuance only. It never runs cert-manager,
// CSI, cainjector, a workload, or an authenticated observer.
func initialControllerFixture(t *testing.T, mode, provider string) (*initialCertificateInstaller, func() *initialCertificateInstaller) {
	t.Helper()
	store, _ := bootstrapStoreFixture(t)
	kube := store.kube.(*kubefake.Clientset)
	selected, err := loader.Load("../../../deploy/Chart")
	require.NoError(t, err)
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{
		"enabled": true, "tls": map[string]any{"provider": provider},
		"certificates": map[string]any{"mode": mode},
	}}}
	options := NewDefaultClusterOptions()
	options.Radius.Timeout = 2 * time.Second
	impl := &Impl{contourCluster: &fakeContourCluster{},
		newRotationStore:     func(string, string) (*rotationStore, error) { return store, nil },
		newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil }}
	if mode != "selfSigned" {
		ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
		settings := certificateValues(values)
		settings["caBundle"] = string(ca)
		if mode == "caSecret" {
			settings["caSecret"] = "operator-ca"
			_, err := kube.CoreV1().Secrets(store.namespace).Create(t.Context(), caTLSSecret("operator-ca", ca, key), metav1.CreateOptions{})
			require.NoError(t, err)
		} else {
			settings["issuerRef"] = map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": "operator-issuer"}
			_, err := store.dynamic.Resource(rotationIssuerResource).Namespace(store.namespace).Create(t.Context(), &unstructured.Unstructured{
				Object: map[string]any{"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
					"metadata": map[string]any{"name": "operator-issuer", "namespace": store.namespace},
					"spec":     map[string]any{"ca": map[string]any{"secretName": "unread-external-key"}}},
			}, metav1.CreateOptions{})
			require.NoError(t, err)
		}
	}
	kube.PrependReactor("get", "secrets", func(action k8stesting.Action) (bool, runtime.Object, error) {
		name := action.(k8stesting.GetAction).GetName()
		if mode != "selfSigned" || name != "radius-ca" && name != "radius-apiserver-ca" {
			return false, nil, nil
		}
		if _, err := kube.Tracker().Get(corev1.SchemeGroupVersion.WithResource("secrets"), store.namespace, name); err == nil {
			return false, nil, nil
		}
		certificate, err := store.dynamic.Resource(rotationCertificateResource).Namespace(store.namespace).Get(t.Context(), name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil, nil
		}
		require.NoError(t, err)
		ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) { c.Subject.CommonName = name }, nil, nil)
		secret := caTLSSecret(name, ca, key)
		secret.Data[caCertificateKey] = ca
		secret.Annotations = map[string]string{"cert-manager.io/certificate-name": name,
			"cert-manager.io/issuer-name": "radius-selfsigned-bootstrap", "cert-manager.io/issuer-kind": "Issuer",
			"cert-manager.io/issuer-group": "cert-manager.io"}
		secret.Namespace = store.namespace
		// Reactors cannot recursively acquire the fake client's lock.
		secret.UID, secret.ResourceVersion = "fixture-"+certificate.GetUID(), "1"
		require.NoError(t, kube.Tracker().Create(corev1.SchemeGroupVersion.WithResource("secrets"), secret, store.namespace))
		certificate.Object["status"] = map[string]any{"conditions": []any{map[string]any{
			"type": "Ready", "status": "True", "observedGeneration": certificate.GetGeneration(),
		}}}
		_, err = store.dynamic.Resource(rotationCertificateResource).Namespace(store.namespace).Update(t.Context(), certificate, metav1.UpdateOptions{})
		require.NoError(t, err)
		return true, secret.DeepCopy(), nil
	})
	prepare := func() *initialCertificateInstaller {
		current := copyValues(values)
		target, plan, err := impl.prepareCertificateTarget(t.Context(), options, selected, current, "")
		require.NoError(t, err)
		installer, err := impl.prepareAggregationBootstrap(t.Context(), options, selected, nil, current, "", plan, target)
		require.NoError(t, err)
		require.NotNil(t, installer)
		installer.applyFixture = func(ctx context.Context) error {
			require.Equal(t, "published", installer.state.Phase)
			return installer.checkMaterial(ctx)
		}
		installer.verifyFixture = func(ctx context.Context) error {
			// This callback verifies orchestration, not installed-product serving.
			return installer.checkMaterial(ctx)
		}
		return installer
	}
	return prepare(), prepare
}

func TestInitialCertificateControllerFixtureExecution(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret", "issuerRef"} {
		for _, provider := range []string{"radius", "external"} {
			t.Run(mode+"/"+provider, func(t *testing.T) {
				b, _ := initialControllerFixture(t, mode, provider)
				require.NoError(t, b.run(t.Context()))
				journal, state, err := readInitialCertificateJournal(t.Context(), b.store)
				require.NoError(t, err)
				require.Equal(t, "complete", state.Phase)
				require.Len(t, state.Sources, len(b.planeNames()))
				require.NotContains(t, journal.Data[rotationJournalKey], "PRIVATE KEY")
				require.NotContains(t, journal.Data[rotationJournalKey], "tls.key")
				if mode == "selfSigned" && provider == "radius" {
					require.NotEqual(t, state.Anchors["internal"].CA, state.Anchors["aggregation"].CA)
				}
				source, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), ucpAPIServerTrustSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				require.Len(t, source.Data, 1)
				require.Equal(t, "true", source.Annotations[allowDirectInjectionAnnotation])
				for _, action := range b.store.kube.(*kubefake.Clientset).Actions() {
					require.NotEqual(t, "token", action.GetSubresource())
					if mode == "issuerRef" {
						if get, ok := action.(k8stesting.GetAction); ok && action.GetResource().Resource == "secrets" {
							require.NotEqual(t, "unread-external-key", get.GetName())
						}
					}
				}
			})
		}
	}
}

func TestInitialCertificateControllerFixtureRecovery(t *testing.T) {
	for _, point := range []string{"namespace intent", "root journal conflict", "source journal conflict", "chart interruption"} {
		t.Run(point, func(t *testing.T) {
			b, retry := initialControllerFixture(t, "selfSigned", "radius")
			injected := false
			kube := b.store.kube.(*kubefake.Clientset)
			kube.PrependReactor("create", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
				object := action.(k8stesting.CreateAction).GetObject().(*corev1.ConfigMap)
				if point == "namespace intent" && !injected && object.Name == initialCertificateJournal {
					injected = true
					return true, nil, errors.New("fixture interruption")
				}
				return false, nil, nil
			})
			kube.PrependReactor("update", "configmaps", func(action k8stesting.Action) (bool, runtime.Object, error) {
				object := action.(k8stesting.UpdateAction).GetObject().(*corev1.ConfigMap)
				if injected || object.Name != initialCertificateJournal {
					return false, nil, nil
				}
				var proposed initialCertificateState
				require.NoError(t, json.Unmarshal([]byte(object.Data[rotationJournalKey]), &proposed))
				if point == "root journal conflict" && len(proposed.Roots) == 1 ||
					point == "source journal conflict" && len(proposed.Sources) == 1 {
					injected = true
					return true, nil, apierrors.NewConflict(corev1.Resource("configmaps"), object.Name, errors.New("fixture interruption"))
				}
				return false, nil, nil
			})
			if point == "chart interruption" {
				b.applyFixture = func(context.Context) error { injected = true; return errors.New("fixture interruption") }
			}
			require.ErrorContains(t, b.run(t.Context()), "fixture interruption")
			require.True(t, injected)
			if point == "namespace intent" {
				off := copyValues(b.values)
				require.NoError(t, unstructured.SetNestedField(off, false, "global", "rbac", "enabled"))
				_, err := b.impl.prepareAggregationBootstrap(t.Context(), b.options, b.selected, nil, off, "", nil, nil)
				require.ErrorContains(t, err, "cannot be bypassed")
			}
			before, err := b.store.kube.CoreV1().Secrets(b.store.namespace).List(t.Context(), metav1.ListOptions{})
			require.NoError(t, err)
			resumed := retry()
			require.NoError(t, resumed.run(t.Context()))
			require.Equal(t, b.state.ID, resumed.state.ID)
			for _, secret := range before.Items {
				live, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), secret.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, secret.UID, live.UID)
				require.Equal(t, secret.Data, live.Data)
			}
		})
	}
}

func TestInitialCertificateRecoveryRejectsChangedMaterial(t *testing.T) {
	for _, change := range []string{"source bytes", "source mode", "signer revision", "namespace UID", "journal unknown field"} {
		t.Run(change, func(t *testing.T) {
			b, retry := initialControllerFixture(t, "caSecret", "external")
			b.applyFixture = func(context.Context) error { return errors.New("fixture pause") }
			require.ErrorContains(t, b.run(t.Context()), "fixture pause")
			ctx := t.Context()
			switch change {
			case "source bytes", "source mode", "signer revision":
				name := ucpAPIServerTrustSecretName
				if change == "signer revision" {
					name = "operator-ca"
				}
				object, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				if change == "source bytes" {
					object.Data[caCertificateKey] = append(object.Data[caCertificateKey], '\n')
				} else {
					if object.Annotations == nil {
						object.Annotations = map[string]string{}
					}
					object.Annotations["radius.io/certificate-mode"] = "changed"
				}
				_, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Update(ctx, object, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "namespace UID":
				object, err := b.store.kube.CoreV1().Namespaces().Get(ctx, b.store.namespace, metav1.GetOptions{})
				require.NoError(t, err)
				object.UID = "replacement"
				_, err = b.store.kube.CoreV1().Namespaces().Update(ctx, object, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "journal unknown field":
				object := b.journal.DeepCopy()
				object.Data[rotationJournalKey] = strings.Replace(object.Data[rotationJournalKey], `"version":1`, `"version":1,"unknown":true`, 1)
				_, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Update(ctx, object, metav1.UpdateOptions{})
				require.NoError(t, err)
				_, _, err = readInitialCertificateJournal(ctx, b.store)
				require.ErrorContains(t, err, "unknown field")
				return
			}
			resumed := retry()
			require.Error(t, resumed.checkPrepared(ctx), "recovery must reject changed material before dependency writes")
			resumed.applyFixture = func(context.Context) error { t.Fatal("changed material must stop before Helm"); return nil }
			require.Error(t, resumed.run(ctx))
		})
	}
}

func TestInitialCertificateCancellationAndLease(t *testing.T) {
	for _, loseLease := range []bool{false, true} {
		t.Run(fmt.Sprint(loseLease), func(t *testing.T) {
			b, _ := initialControllerFixture(t, "caSecret", "external")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			b.applyFixture = func(context.Context) error {
				if loseLease {
					lease, err := b.store.kube.CoordinationV1().Leases(b.store.namespace).Get(ctx, identityOperationLeaseName, metav1.GetOptions{})
					require.NoError(t, err)
					lease.Spec.HolderIdentity = ptr.To("another-operation")
					_, err = b.store.kube.CoordinationV1().Leases(b.store.namespace).Update(ctx, lease, metav1.UpdateOptions{})
					require.NoError(t, err)
				} else {
					cancel()
				}

				return nil
			}
			b.verifyFixture = func(ctx context.Context) error { return b.checkMaterial(ctx) }
			require.Error(t, b.run(ctx))
			_, state, err := readInitialCertificateJournal(t.Context(), b.store)
			require.NoError(t, err)
			require.Equal(t, "published", state.Phase)
		})
	}
}

func TestInitialCertificateTransportCancellation(t *testing.T) {
	t.Run("cancel during write guard", func(t *testing.T) {
		var writes atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { writes.Add(1) }))
		defer server.Close()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		transport := initialCertificateTransport{next: http.DefaultTransport, ctx: ctx,
			check: func(context.Context) error { cancel(); return nil }}
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(request)
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, writes.Load())
	})
	for _, owner := range []string{"operation", "request"} {
		t.Run(owner, func(t *testing.T) {
			operation, cancelOperation := context.WithCancel(t.Context())
			requestContext, cancelRequest := context.WithCancel(t.Context())
			defer cancelOperation()
			defer cancelRequest()
			started := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				close(started)
				<-request.Context().Done()
			}))
			defer server.Close()
			transport := initialCertificateTransport{next: http.DefaultTransport, ctx: operation,
				check: func(context.Context) error { t.Error("read must not invoke write guard"); return nil }}
			request, err := http.NewRequestWithContext(requestContext, http.MethodGet, server.URL, nil)
			require.NoError(t, err)
			finished := make(chan error, 1)
			go func() { _, err := transport.RoundTrip(request); finished <- err }()
			select {
			case <-started:
			case <-time.After(5 * time.Second):
				cancelOperation()
				cancelRequest()
				t.Fatal("fixture transport did not start")
			}
			if owner == "operation" {
				cancelOperation()
			} else {
				cancelRequest()
			}
			require.ErrorIs(t, <-finished, context.Canceled)
		})
	}
}
