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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func emptyRotationTestStore(_ string, namespace string) (*rotationStore, error) {
	return &rotationStore{kube: kubefake.NewClientset(), dynamic: dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()),
		namespace: namespace, release: radiusReleaseName, now: time.Now}, nil
}

func TestRotationIssuedLeaf(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ca, _, parent := testSigningCA(t, key, nil, nil, nil)
	otherCA, _, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, tt := range []struct {
		name string
		edit func(*x509.Certificate)
		ca   []byte
		want string
	}{
		{name: "both usages", ca: ca},
		{name: "another CA", ca: otherCA, want: "does not verify"},
		{name: "expired", ca: ca, edit: func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) }, want: "does not verify"},
		{name: "server only", ca: ca, edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }, want: "does not verify"},
		{name: "wrong identity", ca: ca, edit: func(c *x509.Certificate) { c.DNSNames = []string{"controller.radius-system.svc"} }, want: "wrong Radius identity"},
		{name: "extra identity", ca: ca, edit: func(c *x509.Certificate) { c.DNSNames = append(c.DNSNames, "controller.radius-system.svc") }, want: "wrong Radius identity"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, private, leaf := testSigningCA(t, nil, func(c *x509.Certificate) {
				c.IsCA = false
				c.KeyUsage = x509.KeyUsageDigitalSignature
				c.DNSNames = []string{"ucp.radius-system.svc"}
				c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
				if tt.edit != nil {
					tt.edit(c)
				}
			}, parent, key)
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{UID: types.UID(uuid.NewString())}, Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{"tls.crt": cert, "tls.key": private, "ca.crt": otherCA}}
			digest, err := rotationLeafDigest(secret, tt.ca, RadiusSystemNamespace, "ucp", true, time.Now())
			if tt.want != "" {
				require.ErrorContains(t, err, tt.want)
				return
			}
			require.NoError(t, err)
			sum := sha256.Sum256(leaf.Raw)
			require.Equal(t, hex.EncodeToString(sum[:]), digest, "leaf Secret ca.crt never grants trust")
			_, secret.Data["tls.key"], _ = testSigningCA(t, nil, nil, nil, nil)
			_, err = rotationLeafDigest(secret, ca, RadiusSystemNamespace, "ucp", true, time.Now())
			require.ErrorContains(t, err, "matching pair")
		})
	}
}

func TestRotationAggregationProbe(t *testing.T) {
	t.Parallel()
	id := types.UID(uuid.NewString())
	for _, tt := range []struct {
		name   string
		status int
		uid    string
		want   string
	}{
		{"new backend", 200, string(id), ""},
		{"old pooled backend", 200, uuid.NewString(), "expected new UCP"},
		{"missing capability", 200, "", "expected new UCP"},
		{"authorization rejected", 403, string(id), "HTTP 403"},
		{"unavailable", 503, "", "HTTP 503"},
		{"redirect rejected", 302, string(id), "HTTP 302"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/apis/api.ucp.dev/v1alpha3/planes/radius/local", r.URL.Path)
				require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
				require.NotEmpty(t, r.URL.Query().Get("api-version"))
				w.Header().Set(mtls.AggregationPodUIDHeader, tt.uid)
				w.Header().Set("Location", "/untrusted")
				w.WriteHeader(tt.status)
			}))
			defer server.Close()
			err := observeAggregatedPod(t.Context(), &rest.Config{Host: server.URL, BearerToken: "test-token"}, map[types.UID]bool{id: true})
			if tt.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.want)
			}
		})
	}
}

func TestRotationRequiresNewSigningKey(t *testing.T) {
	t.Parallel()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	old, _, _ := testSigningCA(t, key, nil, nil, nil)
	renewed, _, _ := testSigningCA(t, key, func(c *x509.Certificate) { c.Subject.CommonName = "renewed-radius-ca" }, nil, nil)
	next, _, _ := testSigningCA(t, nil, nil, nil, nil)
	require.ErrorContains(t, rotationCheckRootKeys(old, renewed), "different signing key")
	require.NoError(t, rotationCheckRootKeys(old, next))
	require.NoError(t, rotationCheckRootKeys(old, old), "aggregation migration can retain the identical component CA")
	legacy, _, _ := testSigningCA(t, nil, nil, nil, nil)
	interrupted := rotationState{ID: uuid.NewString(), OldCA: legacy, NewCA: next, Plan: &rotationPlan{OldComponentCA: old}}
	require.ErrorContains(t, checkRecoveryTarget(renewed, interrupted), "different signing key")
	require.ErrorContains(t, checkRecoveryTarget(old, interrupted), "cannot restore")
	require.NoError(t, checkRecoveryTarget(next, interrupted), "the interrupted target remains resumable")
}

func TestRotationSelfSignedSnapshot(t *testing.T) {
	t.Parallel()
	certificate, private, _ := testSigningCA(t, nil, nil, nil, nil)
	options := NewDefaultClusterOptions()
	root := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "radius-ca", Namespace: options.Radius.Namespace},
		Type: corev1.SecretTypeTLS, Data: map[string][]byte{"tls.crt": certificate, "tls.key": private}}
	kube := kubefake.NewClientset(root)
	impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil }}
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	plan, public, name, mode, err := impl.rotationTarget(t.Context(), options, values, "")
	require.NoError(t, err)
	require.Equal(t, "selfSigned", mode)
	require.Equal(t, certificate, public)
	require.NotEqual(t, root.Name, name)
	for _, action := range kube.Actions() {
		require.Equal(t, "get", action.GetVerb(), "preparation is read-only")
	}
	require.NoError(t, impl.applyCertificatePlan(t.Context(), plan))
	imported, err := kube.CoreV1().Secrets(root.Namespace).Get(t.Context(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, root.Data, imported.Data)
	require.Empty(t, imported.OwnerReferences)
	require.Empty(t, imported.Annotations)
	retry, _, sameName, _, err := impl.rotationTarget(t.Context(), options, values, "")
	require.NoError(t, err)
	require.Equal(t, name, sameName)
	require.Nil(t, retry.secret)
	imported.Data["tls.key"] = []byte("changed")
	_, err = kube.CoreV1().Secrets(root.Namespace).Update(t.Context(), imported, metav1.UpdateOptions{})
	require.NoError(t, err)
	_, _, _, _, err = impl.rotationTarget(t.Context(), options, values, "")
	require.ErrorContains(t, err, "will not be overwritten")
}
