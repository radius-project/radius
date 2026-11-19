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
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	"helm.sh/helm/v4/pkg/storage/driver"
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
	store := bootstrapStoreFixture(t)
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
		ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
			c.Subject.CommonName = name
			c.NotBefore, c.NotAfter = store.now().Add(-time.Hour), store.now().Add(365*24*time.Hour)
		}, nil, nil)
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

func renewBootstrapFixture(t *testing.T, b *initialCertificateInstaller, change func(*x509.Certificate, *corev1.Secret)) {
	t.Helper()
	future := b.store.now()
	for _, plane := range b.planeNames() {
		anchor := b.state.Anchors[plane]
		secret, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), anchor.Signer.Name, metav1.GetOptions{})
		require.NoError(t, err)
		old := mustParseCertificate(t, anchor.CA)
		if old.NotAfter.After(future) {
			future = old.NotAfter.Add(time.Minute)
		}
		block, _ := pem.Decode(secret.Data[corev1.TLSPrivateKeyKey])
		require.NotNil(t, block)
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		require.NoError(t, err)
		signer, ok := key.(crypto.Signer)
		require.True(t, ok)
		next := *old
		next.SerialNumber = big.NewInt(2)
		next.NotBefore, next.NotAfter = old.NotAfter.Add(-time.Hour), old.NotAfter.Add(24*time.Hour)
		if change != nil {
			change(&next, secret)
		}
		der, err := x509.CreateCertificate(rand.Reader, &next, &next, signer.Public(), signer)
		require.NoError(t, err)
		bundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
		secret.Data[corev1.TLSCertKey], secret.Data[caCertificateKey] = bundle, bundle
		_, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Update(t.Context(), secret, metav1.UpdateOptions{})
		require.NoError(t, err)
	}
	b.store.now = func() time.Time { return future }
}

func TestInitialCertificateRenewalAndUninstall(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		for _, version := range []int{1, 2} {
			t.Run(fmt.Sprintf("%s/schema%d", provider, version), func(t *testing.T) {
				b, prepare := initialControllerFixture(t, "selfSigned", provider)
				require.NoError(t, b.run(t.Context()))
				require.Equal(t, 2, b.state.Version)
				if version == 1 {
					b.state.Version = 1
					for plane, anchor := range b.state.Anchors {
						anchor.PublicKey, anchor.Subject = nil, nil
						b.state.Anchors[plane] = anchor
					}
					data, err := json.Marshal(b.state)
					require.NoError(t, err)
					b.journal.Data[rotationJournalKey] = string(data)
					b.journal, err = b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Update(t.Context(), b.journal, metav1.UpdateOptions{})
					require.NoError(t, err)
				}
				renewBootstrapFixture(t, b, nil)
				require.NoError(t, b.checkObservedMaterial(t.Context()))
				for _, anchor := range b.state.Anchors {
					require.Error(t, b.store.checkCertificateSignerSnapshot(t.Context(), anchor.Signer, rotationPublish),
						"an active lifecycle must still reject the changed revision and certificate")
				}
				for _, anchor := range b.state.Anchors {
					_, err := validatePublicCA(anchor.CA, b.store.now())
					require.Error(t, err, "the historical anchor has expired")
					live, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), anchor.Signer.Name, metav1.GetOptions{})
					require.NoError(t, err)
					require.NotEqual(t, anchor.Signer.ResourceVersion, live.ResourceVersion)
				}
				_, _, err := readInitialCertificateJournal(t.Context(), b.store)
				require.NoError(t, err)
				initialUninstallFixture(t, b, nil)
				require.NoError(t, b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), ""))
				fresh := prepare()
				require.NoError(t, fresh.run(t.Context()))
				require.NotEqual(t, b.state.ID, fresh.state.ID)
			})
		}
	}
}

func TestInitialCertificateRenewalRejectsChangedIdentity(t *testing.T) {
	for _, tt := range []struct {
		name string
		edit func(*x509.Certificate, *corev1.Secret)
	}{
		{"subject", func(c *x509.Certificate, _ *corev1.Secret) {
			c.RawSubject = nil
			c.Subject.Organization = []string{"foreign"}
		}},
		{"constraints", func(c *x509.Certificate, _ *corev1.Secret) { c.MaxPathLen = 0; c.MaxPathLenZero = true }},
		{"UID", func(_ *x509.Certificate, s *corev1.Secret) { s.UID = "replacement" }},
		{"type", func(_ *x509.Certificate, s *corev1.Secret) { s.Type = corev1.SecretTypeOpaque }},
		{"owner", func(_ *x509.Certificate, s *corev1.Secret) { s.Annotations[certificateNameAnnotation] = "foreign" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := initialControllerFixture(t, "selfSigned", "radius")
			require.NoError(t, b.run(t.Context()))
			renewBootstrapFixture(t, b, tt.edit)
			require.Error(t, b.checkObservedMaterial(t.Context()))
			initialUninstallFixture(t, b, nil)
			require.Error(t, b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), ""))
			_, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(t.Context(), initialCertificateJournal, metav1.GetOptions{})
			require.NoError(t, err, "failed cleanup must retain recovery state")
		})
	}
}

func TestInitialCertificateRenewalRejectsNewKey(t *testing.T) {
	b, _ := initialControllerFixture(t, "selfSigned", "radius")
	require.NoError(t, b.run(t.Context()))
	anchor := b.state.Anchors["internal"]
	secret, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), anchor.Signer.Name, metav1.GetOptions{})
	require.NoError(t, err)
	ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.Subject.CommonName = anchor.Signer.Name
	}, nil, nil)
	secret.Data[corev1.TLSCertKey], secret.Data[corev1.TLSPrivateKeyKey], secret.Data[caCertificateKey] = ca, key, ca
	_, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Update(t.Context(), secret, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.ErrorContains(t, b.checkObservedMaterial(t.Context()), "key or subject changed")
}

// Helm removal and certificate issuance are fixtures; the uninstall entrypoint,
// bootstrap execution, ownership checks and cleanup use their production paths.
func initialUninstallFixture(t *testing.T, b *initialCertificateInstaller, afterHelm func()) {
	t.Helper()
	client := NewMockHelmClient(gomock.NewController(t))
	b.impl.Helm = client
	b.impl.removeDefaultContourGateway = (&defaultContourGatewayCalls{}).remove
	client.EXPECT().RunHelmUninstall(gomock.Any(), radiusReleaseName, b.store.namespace, true).
		DoAndReturn(func(*helm.Configuration, string, string, bool) (*release.UninstallReleaseResponse, error) {
			for key := range b.state.Roots {
				resource, name, err := initialRootResource(key)
				require.NoError(t, err)
				err = b.store.dynamic.Resource(resource).Namespace(b.store.namespace).Delete(t.Context(), name, metav1.DeleteOptions{})
				require.True(t, err == nil || apierrors.IsNotFound(err), "%v", err)
			}
			if afterHelm != nil {
				afterHelm()
			}
			return &release.UninstallReleaseResponse{}, nil
		})
	client.EXPECT().RunHelmUninstall(gomock.Any(), contourReleaseName, b.store.namespace, true).
		Return(&release.UninstallReleaseResponse{}, nil).AnyTimes()
	for _, name := range []string{certManagerReleaseName} {
		client.EXPECT().RunHelmGet(gomock.Any(), name).Return(nil, driver.ErrReleaseNotFound).AnyTimes()
	}
}

func TestInitialCertificateUninstallAndFreshEnforce(t *testing.T) {
	for _, mode := range []string{"selfSigned", "caSecret"} {
		t.Run(mode, func(t *testing.T) {
			b, fresh := initialControllerFixture(t, mode, "radius")
			require.NoError(t, b.run(t.Context()))
			var operator *corev1.Secret
			if mode == "caSecret" {
				var err error
				operator, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), "operator-ca", metav1.GetOptions{})
				require.NoError(t, err)
			}
			initialUninstallFixture(t, b, nil)
			require.NoError(t, b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context"))
			_, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(t.Context(), initialCertificateJournal, metav1.GetOptions{})
			require.True(t, apierrors.IsNotFound(err))
			namespace, err := b.store.kube.CoreV1().Namespaces().Get(t.Context(), b.store.namespace, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotContains(t, namespace.Annotations, initialCertificateIntent)
			if operator != nil {
				current, err := b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), operator.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, operator, current)
			}
			next := fresh()
			require.NoError(t, next.run(t.Context()))
			require.Equal(t, "complete", next.state.Phase)
			require.NotEqual(t, b.state.ID, next.state.ID)
		})
	}
}

func TestInitialCertificateUninstallPreservesForeignState(t *testing.T) {
	for _, change := range []string{"journal", "namespace intent", "public source", "signer"} {
		t.Run(change, func(t *testing.T) {
			b, _ := initialControllerFixture(t, "selfSigned", "radius")
			require.NoError(t, b.run(t.Context()))
			ctx := t.Context()
			var preserved metav1.Object
			switch change {
			case "journal", "public source":
				name := initialCertificateJournal
				if change == "public source" {
					name = "radius-mtls-ca"
				}
				client := b.store.kube.CoreV1().ConfigMaps(b.store.namespace)
				cm, err := client.Get(ctx, name, metav1.GetOptions{})
				require.NoError(t, err)
				if change == "journal" {
					cm.Labels[rotationOwner] = "foreign"
				} else {
					cm.Annotations[initialCertificateIntent] = "foreign"
				}
				preserved, err = client.Update(ctx, cm, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "namespace intent":
				client := b.store.kube.CoreV1().Namespaces()
				ns, err := client.Get(ctx, b.store.namespace, metav1.GetOptions{})
				require.NoError(t, err)
				ns.Annotations[initialCertificateIntent] = "foreign"
				preserved, err = client.Update(ctx, ns, metav1.UpdateOptions{})
				require.NoError(t, err)
			case "signer":
				client := b.store.kube.CoreV1().Secrets(b.store.namespace)
				secret, err := client.Get(ctx, "radius-ca", metav1.GetOptions{})
				require.NoError(t, err)
				secret.Annotations["cert-manager.io/certificate-name"] = "foreign"
				preserved, err = client.Update(ctx, secret, metav1.UpdateOptions{})
				require.NoError(t, err)
			}
			if change == "journal" {
				b.impl.Helm = NewMockHelmClient(gomock.NewController(t))
			} else {
				initialUninstallFixture(t, b, nil)
			}
			require.Error(t, b.impl.UninstallRadius(ctx, NewDefaultClusterOptions(), "test-context"))
			var current metav1.Object
			var err error
			switch change {
			case "journal", "public source":
				current, err = b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(ctx, preserved.GetName(), metav1.GetOptions{})
			case "namespace intent":
				current, err = b.store.kube.CoreV1().Namespaces().Get(ctx, preserved.GetName(), metav1.GetOptions{})
			case "signer":
				current, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Get(ctx, preserved.GetName(), metav1.GetOptions{})
			}
			require.NoError(t, err)
			require.Equal(t, preserved, current)
			_, err = b.impl.prepareAggregationBootstrap(ctx, b.options, b.selected, nil, b.values, "", b.importPlan, b.target)
			require.Error(t, err, "fresh enforce must not adopt unknown remaining state")
		})
	}
}

func TestInitialCertificateUninstallInterruptedCleanup(t *testing.T) {
	for _, point := range []string{"radius-ca", "radius-apiserver-ca", "radius-mtls-ca", "ucp-spiffe", ucpAPIServerTrustSecretName, "namespace", initialCertificateJournal} {
		t.Run(point, func(t *testing.T) {
			b, fresh := initialControllerFixture(t, "selfSigned", "radius")
			require.NoError(t, b.run(t.Context()))
			kube := b.store.kube.(*kubefake.Clientset)
			_, seedErr := kube.CoreV1().Secrets(b.store.namespace).Create(t.Context(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ucp-spiffe", Namespace: b.store.namespace, Annotations: map[string]string{certificateNameAnnotation: "ucp-spiffe"}}}, metav1.CreateOptions{})
			require.NoError(t, seedErr)
			injected := false
			kube.PrependReactor("*", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
				match := false
				switch action := action.(type) {
				case k8stesting.DeleteAction:
					if action.GetResource().Resource != "leases" {
						options := action.GetDeleteOptions()
						require.NotNil(t, options.Preconditions)
						require.NotEmpty(t, *options.Preconditions.UID)
						require.NotEmpty(t, *options.Preconditions.ResourceVersion)
					}
					match = action.GetName() == point
				case k8stesting.PatchAction:
					match = point == "namespace" && action.GetResource().Resource == "namespaces"
				}
				if match && !injected {
					injected = true
					return true, nil, errors.New("cleanup interruption")
				}
				return false, nil, nil
			})
			initialUninstallFixture(t, b, nil)
			require.ErrorContains(t, b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context"), "cleanup interruption")
			require.True(t, injected)
			_, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(t.Context(), initialCertificateJournal, metav1.GetOptions{})
			require.NoError(t, err)
			initialUninstallFixture(t, b, nil)
			require.NoError(t, b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context"))
			require.NoError(t, fresh().run(t.Context()))
		})
	}
}

func TestInitialCertificateUninstallCancellation(t *testing.T) {
	b, _ := initialControllerFixture(t, "selfSigned", "radius")
	require.NoError(t, b.run(t.Context()))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	initialUninstallFixture(t, b, cancel)
	require.ErrorIs(t, b.impl.UninstallRadius(ctx, NewDefaultClusterOptions(), "test-context"), context.Canceled)
	_, err := b.store.kube.CoreV1().ConfigMaps(b.store.namespace).Get(t.Context(), initialCertificateJournal, metav1.GetOptions{})
	require.NoError(t, err)
	_, err = b.store.kube.CoreV1().Secrets(b.store.namespace).Get(t.Context(), "radius-ca", metav1.GetOptions{})
	require.NoError(t, err)
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

type countedRequestBody struct {
	io.Reader
	closes int
}

func (b *countedRequestBody) Close() error {
	b.closes++
	return nil
}

type bootstrapRoundTripFunc func(*http.Request) (*http.Response, error)

func (f bootstrapRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestInitialCertificateTransportRequestBody(t *testing.T) {
	guardErr := errors.New("mutation rejected")
	for _, scenario := range []string{"request canceled before guard", "operation canceled before guard", "guard error",
		"operation canceled in guard", "request canceled in guard", "delegated"} {
		t.Run(scenario, func(t *testing.T) {
			operation, cancelOperation := context.WithCancel(t.Context())
			defer cancelOperation()
			ctx, cancelRequest := context.WithCancel(t.Context())
			defer cancelRequest()
			switch scenario {
			case "request canceled before guard":
				cancelRequest()
			case "operation canceled before guard":
				cancelOperation()
			}
			body := &countedRequestBody{Reader: strings.NewReader("payload")}
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://kubernetes/", body)
			require.NoError(t, err)
			transport := initialCertificateTransport{ctx: operation, check: func(context.Context) error {
				switch scenario {
				case "guard error":
					return guardErr
				case "operation canceled in guard":
					cancelOperation()
				case "request canceled in guard":
					cancelRequest()
				}
				return nil
			}, next: bootstrapRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				require.Equal(t, "delegated", scenario)
				require.Zero(t, body.closes)
				require.NoError(t, r.Body.Close())
				return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
			})}
			resp, err := transport.RoundTrip(req)
			switch scenario {
			case "delegated":
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
			case "guard error":
				require.ErrorIs(t, err, guardErr)
			default:
				require.ErrorIs(t, err, context.Canceled)
			}
			require.Equal(t, 1, body.closes)
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

func uninstallTestStore(t *testing.T) func(string, string) (*rotationStore, error) {
	t.Helper()
	store := bootstrapStoreFixture(t)
	_, err := store.kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: store.namespace}}, metav1.CreateOptions{})
	require.NoError(t, err)
	return func(_ string, namespace string) (*rotationStore, error) {
		require.Equal(t, store.namespace, namespace)
		return store, nil
	}
}

func TestUninstallExclusionBeforeHelm(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider-after-lease=%t", late), func(t *testing.T) {
			b, _ := initialControllerFixture(t, "selfSigned", "radius")
			kube := b.store.kube.(*kubefake.Clientset)
			namespace := b.store.namespace
			_, err := kube.CoreV1().Namespaces().Create(t.Context(), &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
			require.NoError(t, err)
			journal := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: identityProviderJournal, Namespace: namespace}}
			if late {
				kube.PrependReactor("create", "leases", func(k8stesting.Action) (bool, runtime.Object, error) {
					return false, nil, kube.Tracker().Add(journal)
				})
			} else {
				require.NoError(t, kube.Tracker().Add(journal))
			}
			client := NewMockHelmClient(gomock.NewController(t))
			b.impl.Helm = client
			stop := errors.New("Helm reached before provider journal rejection")
			client.EXPECT().RunHelmUninstall(gomock.Any(), radiusReleaseName, namespace, true).Return(nil, stop).AnyTimes()
			err = b.impl.UninstallRadius(t.Context(), NewDefaultClusterOptions(), "test-context")
			require.NotErrorIs(t, err, stop)
			require.ErrorContains(t, err, identityProviderJournal)
		})
	}
}
