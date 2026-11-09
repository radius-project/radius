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
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	helm "helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	"helm.sh/helm/v4/pkg/release"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func testSigningCA(t *testing.T, signer crypto.Signer, edit func(*x509.Certificate), parent *x509.Certificate, parentKey crypto.Signer) ([]byte, []byte, *x509.Certificate) {
	t.Helper()
	if signer == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)
		signer = key
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "dedicated-radius-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	if edit != nil {
		edit(template)
	}
	if parent == nil {
		parent, parentKey = template, signer
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, signer.Public(), parentKey)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	key, err := x509.MarshalPKCS8PrivateKey(signer)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), cert
}

func TestValidateSigningCA(t *testing.T) {
	t.Parallel()
	valid, key, _ := testSigningCA(t, nil, nil, nil, nil)
	tampered, _ := pem.Decode(valid)
	tampered.Bytes[len(tampered.Bytes)-1] ^= 1
	_, otherKey, _ := testSigningCA(t, nil, nil, nil, nil)
	tests := []struct {
		name string
		cert []byte
		key  []byte
		err  string
	}{
		{name: "valid P256", cert: valid, key: key},
		{name: "surrounding whitespace", cert: append([]byte("\n "), valid...), key: key},
		{name: "empty certificate", key: key, err: "empty"},
		{name: "junk certificate", cert: []byte("secret-sentinel"), key: key, err: "only PEM"},
		{name: "malformed PEM", cert: []byte("-----BEGIN CERTIFICATE-----\ninvalid\n"), key: key, err: "malformed PEM"},
		{name: "malformed block before certificate", cert: append([]byte("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"), valid...), key: key, err: "malformed PEM"},
		{name: "malformed DER", cert: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("secret-sentinel")}), key: key, err: "X.509"},
		{name: "private key in certificate", cert: append(append([]byte{}, valid...), key...), key: key, err: "only PEM"},
		{name: "invalid self signature", cert: pem.EncodeToMemory(tampered), key: key, err: "invalid or unsupported signature"},
		{name: "mismatched key", cert: valid, key: otherKey, err: "matching"},
		{name: "missing key", cert: valid, err: "one unencrypted"},
		{name: "malformed key", cert: valid, key: []byte("secret-sentinel"), err: "one unencrypted"},
		{name: "junk before key", cert: valid, key: append([]byte("secret-sentinel\n"), key...), err: "one unencrypted"},
		{name: "malformed block before key", cert: valid, key: append([]byte("-----BEGIN PRIVATE KEY-----\ninvalid\n-----END PRIVATE KEY-----\n"), key...), err: "one unencrypted"},
		{name: "multiple keys", cert: valid, key: append(append([]byte{}, key...), key...), err: "one unencrypted"},
		{name: "encrypted key", cert: valid, key: pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte("private-key-sentinel")}), err: "matching"},
	}
	for _, invalid := range []struct {
		name string
		edit func(*x509.Certificate)
		err  string
	}{
		{"not a CA", func(c *x509.Certificate) { c.IsCA = false }, "basic constraints"},
		{"missing basic constraints", func(c *x509.Certificate) { c.BasicConstraintsValid = false }, "basic constraints"},
		{"missing signing usage", func(c *x509.Certificate) { c.KeyUsage = x509.KeyUsageDigitalSignature }, "signing key usage"},
		{"expired", func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) }, "not currently valid"},
		{"not yet valid", func(c *x509.Certificate) { c.NotBefore = time.Now().Add(time.Hour) }, "not currently valid"},
		{"unknown critical extension", func(c *x509.Certificate) {
			c.ExtraExtensions = []pkix.Extension{{Id: []int{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0}}}
		}, "unsupported critical"},
	} {
		cert, privateKey, _ := testSigningCA(t, nil, invalid.edit, nil, nil)
		tests = append(tests, struct {
			name string
			cert []byte
			key  []byte
			err  string
		}{invalid.name, cert, privateKey, invalid.err})
	}
	for _, algorithm := range []struct {
		name string
		key  func() (crypto.Signer, error)
		err  string
	}{
		{"P384", func() (crypto.Signer, error) { return ecdsa.GenerateKey(elliptic.P384(), rand.Reader) }, ""},
		{"P521", func() (crypto.Signer, error) { return ecdsa.GenerateKey(elliptic.P521(), rand.Reader) }, "P-256 or P-384"},
		{"RSA2048", func() (crypto.Signer, error) { return rsa.GenerateKey(rand.Reader, 2048) }, ""},
		{"RSA1024", func() (crypto.Signer, error) { return rsa.GenerateKey(rand.Reader, 1024) }, "at least 2048"},
		{"Ed25519", func() (crypto.Signer, error) { _, key, err := ed25519.GenerateKey(rand.Reader); return key, err }, "must use ECDSA"},
	} {
		signer, err := algorithm.key()
		require.NoError(t, err)
		cert, privateKey, _ := testSigningCA(t, signer, nil, nil, nil)
		tests = append(tests, struct {
			name string
			cert []byte
			key  []byte
			err  string
		}{algorithm.name, cert, privateKey, algorithm.err})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cert, err := validateSigningCA(tt.cert, tt.key, time.Now())
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.NotContains(t, err.Error(), "secret-sentinel")
				require.Nil(t, cert)
			} else {
				require.NoError(t, err)
				require.True(t, cert.IsCA)
			}
		})
	}
}

func TestValidateSigningCAChain(t *testing.T) {
	t.Parallel()
	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	rootPEM, _, root := testSigningCA(t, rootKey, nil, nil, nil)
	caPEM, key, ca := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.MaxPathLenZero = true
		c.PermittedDNSDomainsCritical = true
		c.PermittedDNSDomains = []string{RadiusSystemNamespace + ".svc"}
	}, root, rootKey)
	wrongPEM, _, _ := testSigningCA(t, nil, nil, nil, nil)
	wrongNamePEM, _, _ := testSigningCA(t, rootKey, func(c *x509.Certificate) { c.Subject.CommonName = "another-ca" }, nil, nil)
	expiredPEM, _, _ := testSigningCA(t, rootKey, func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) }, nil, nil)
	limitedPEM, _, _ := testSigningCA(t, rootKey, func(c *x509.Certificate) { c.MaxPathLenZero = true }, nil, nil)
	for _, tt := range []struct {
		name   string
		parent []byte
		err    string
	}{
		{"valid parent", rootPEM, ""},
		{"wrong parent", wrongPEM, "issuance parents"},
		{"wrong parent name with same key", wrongNamePEM, "issuance parents"},
		{"expired parent", expiredPEM, "not currently valid"},
		{"path length zero parent", limitedPEM, "path length"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := validateSigningCA(append(append([]byte{}, caPEM...), tt.parent...), key, time.Now())
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, ca.Raw, cert.Raw)
			require.False(t, sameCACertificate(string(rootPEM), ca))
			require.False(t, sameCACertificate(string(caPEM)+string(rootPEM), ca))
			require.False(t, sameCACertificate("-----BEGIN CERTIFICATE-----\ninvalid\n-----END CERTIFICATE-----\n"+string(caPEM), ca))
			require.True(t, sameCACertificate("\n "+string(caPEM)+"\n", ca))
		})
	}
}

func TestCertificateExpiryWarning(t *testing.T) {
	t.Parallel()
	for _, days := range []int{29, 30, 31} {
		t.Run(fmt.Sprintf("%d days", days), func(t *testing.T) {
			t.Parallel()
			cert, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
				c.NotAfter = time.Now().Add(time.Duration(days) * 24 * time.Hour)
			}, nil, nil)
			var logs strings.Builder
			kube := k8sfake.NewClientset(caTLSSecret("operator-ca", cert, key))
			impl := &Impl{
				newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil },
				logger:               func(format string, values ...any) { fmt.Fprintf(&logs, format, values...) }, newRotationStore: emptyRotationTestStore,
			}
			_, err := impl.prepareCertificates(t.Context(), NewDefaultClusterOptions(), nil, caReleaseValues("operator-ca", cert), "", false)
			require.NoError(t, err)
			require.Equal(t, days <= 30, strings.Contains(logs.String(), "within 30 days"))
		})
	}
}

func TestExternalCertificateOffPreservesInstalledTrust(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, mode := range []string{"caSecret", "issuerRef"} {
		for _, scenario := range []string{"omitted", "changed trust", "changed signer", "explicit flag", "fresh"} {
			t.Run(mode+"/"+scenario, func(t *testing.T) {
				t.Parallel()
				stored := externalProviderValues()
				if mode == "caSecret" {
					setCertificateValues(stored, certificateValues(caReleaseValues("operator-ca", cert)))
				} else {
					fixture := newIssuerPolicyFixture(t)
					settings := certificateValues(fixture.values)
					settings["caBundle"] = string(cert)
					setCertificateValues(stored, settings)
				}
				options := NewDefaultClusterOptions()
				options.RadiusReleaseValues = stored
				current := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": false}}}
				switch scenario {
				case "changed trust":
					setCertificateValues(current, map[string]any{"caBundle": "not the installed trust"})
				case "changed signer":
					if mode == "caSecret" {
						setCertificateValues(current, map[string]any{"caSecret": "different-ca"})
					} else {
						setCertificateValues(current, map[string]any{"issuerRef": map[string]any{"name": "different-issuer"}})
					}
				case "explicit flag":
					if mode == "caSecret" {
						options.Certificates.CASecret = "operator-ca"
					} else {
						options.Certificates.CertIssuer = "Issuer/external-ca"
					}
				case "fresh":
					delete(rbacSettings(stored), "mtls")
					delete(stored, "de")
				}
				kube := k8sfake.NewClientset(caTLSSecret("operator-ca", cert, key))
				impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil }}
				_, err := impl.prepareCertificates(t.Context(), options, nil, current, "", scenario != "fresh")
				if scenario == "omitted" {
					require.NoError(t, err)
					require.Equal(t, mode, certificateMode(current))
				} else {
					require.Error(t, err)
				}
				for _, action := range kube.Actions() {
					require.Equal(t, "get", action.GetVerb(), "preparation must not mutate cluster state")
				}
			})
		}
	}
}

func TestCertificateSecretValidationErrors(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, scenario := range []string{"wrong Secret type", "missing key", "API failure"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			secret := caTLSSecret("operator-ca", cert, key)
			if scenario == "wrong Secret type" {
				secret.Type = corev1.SecretTypeOpaque
			} else if scenario == "missing key" {
				delete(secret.Data, corev1.TLSPrivateKeyKey)
			}
			kube := k8sfake.NewClientset(secret)
			if scenario == "API failure" {
				kube.PrependReactor("get", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("private-key-sentinel")
				})
			}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil }, newRotationStore: emptyRotationTestStore}
			_, err := impl.prepareCertificates(t.Context(), NewDefaultClusterOptions(), nil, caReleaseValues("operator-ca", cert), "", false)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-key-sentinel")
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestCertificateOptionsValidate(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		args []string
		err  string
	}{
		{"omitted", nil, ""},
		{"files", []string{"--ca-cert=cert.pem", "--ca-key=key.pem"}, ""},
		{"secret", []string{"--ca-secret=operator-ca"}, ""},
		{"missing key", []string{"--ca-cert=cert.pem"}, "together"},
		{"missing cert", []string{"--ca-key=key.pem"}, "together"},
		{"conflict", []string{"--ca-cert=cert.pem", "--ca-key=key.pem", "--ca-secret=operator-ca"}, "mutually exclusive"},
		{"issuer conflict", []string{"--ca-secret=operator-ca", "--cert-issuer=issuer"}, "mutually exclusive"},
		{"issuer syntax", []string{"--cert-issuer=Issuer/issuer"}, ""},
		{"issuer with public bundle", []string{"--cert-issuer=ClusterIssuer/issuer", "--ca-bundle=public.pem"}, ""},
		{"bundle without issuer", []string{"--ca-bundle=public.pem"}, "requires --cert-issuer"},
		{"bundle with CA Secret", []string{"--ca-bundle=public.pem", "--ca-secret=operator-ca"}, "requires --cert-issuer"},
		{"bare issuer", []string{"--cert-issuer=issuer"}, "must be Issuer/name"},
		{"empty bundle", []string{"--cert-issuer=Issuer/issuer", "--ca-bundle="}, "must not be empty"},
		{"empty secret", []string{"--ca-secret="}, "must not be empty"},
		{"empty issuer", []string{"--cert-issuer="}, "must not be empty"},
		{"empty files", []string{"--ca-cert=", "--ca-key="}, "must not be empty"},
		{"invalid name", []string{"--ca-secret=Not/A/Name"}, "valid Kubernetes Secret name"},
		{"reserved name", []string{"--ca-secret=radius-ca"}, "Radius-managed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			options := CertificateOptions{}
			flags := pflag.NewFlagSet("certificates", pflag.ContinueOnError)
			options.AddFlags(flags)
			require.NoError(t, flags.Parse(tt.args))
			err := options.ValidateFlags(flags)
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.err)
			}
		})
	}
}

func TestParseCertificateIssuer(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		value string
		want  certificateIssuerReference
	}{
		{value: "Issuer/radius-signing", want: certificateIssuerReference{Group: "cert-manager.io", Kind: "Issuer", Name: "radius-signing"}},
		{value: "ClusterIssuer/dedicated.example", want: certificateIssuerReference{Group: "cert-manager.io", Kind: "ClusterIssuer", Name: "dedicated.example"}},
		{value: ""},
		{value: "radius-signing"},
		{value: "Issuer/"},
		{value: "issuer/name"},
		{value: "Issuer/namespace/name"},
		{value: "OtherIssuer/name"},
		{value: "cert-manager.io/Issuer/name"},
		{value: "Issuer/Invalid"},
		{value: "Issuer/*"},
		{value: " Issuer/name"},
		{value: "Issuer/name "},
		{value: "Issuer/" + strings.Repeat("a", 254)},
	} {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()
			got, err := parseCertificateIssuer(tt.value)
			if tt.want.Name == "" {
				require.ErrorContains(t, err, "must be Issuer/name or ClusterIssuer/name")
				require.Empty(t, got)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, got)
			}
		})
	}
}

func TestValidatePublicCA(t *testing.T) {
	t.Parallel()
	rootKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	rootPEM, _, root := testSigningCA(t, rootKey, nil, nil, nil)
	intermediatePEM, privateKey, intermediate := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.Subject.CommonName = "dedicated-radius-intermediate"
	}, root, rootKey)
	expired, _, _ := testSigningCA(t, nil, func(c *x509.Certificate) { c.NotAfter = time.Now().Add(-time.Minute) }, nil, nil)
	notCA, _, _ := testSigningCA(t, nil, func(c *x509.Certificate) { c.IsCA = false }, nil, nil)
	for _, tt := range []struct {
		name string
		pem  []byte
		err  string
	}{
		{"dedicated root public certificate", rootPEM, ""},
		{"dedicated intermediate without key or parent", intermediatePEM, ""},
		{"surrounding whitespace", append([]byte(" \n"), intermediatePEM...), ""},
		{"chain prohibited", append(append([]byte{}, intermediatePEM...), rootPEM...), "exactly one dedicated"},
		{"private key prohibited", privateKey, "only PEM certificates"},
		{"private key appended", append(append([]byte{}, intermediatePEM...), privateKey...), "only PEM certificates"},
		{"expired", expired, "not currently valid"},
		{"not a CA", notCA, "CA basic constraints"},
		{"empty input", nil, "empty"},
		{"malformed input", []byte("private-key-sentinel"), "only PEM certificates"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cert, err := validatePublicCA(tt.pem, time.Now())
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.NotContains(t, err.Error(), "private-key-sentinel")
				return
			}
			require.NoError(t, err)
			require.True(t, cert.IsCA)
			if tt.name != "dedicated root public certificate" {
				require.Equal(t, intermediate.Raw, cert.Raw)
			}
		})
	}
}

func caReleaseValues(name string, cert []byte) map[string]any {
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	setCertificateValues(values, map[string]any{"mode": "caSecret", "caSecret": name, "caBundle": strings.TrimSpace(string(cert))})
	return values
}

func caTLSSecret(name string, cert, key []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: RadiusSystemNamespace},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: cert, corev1.TLSPrivateKeyKey: key},
	}
}

func TestPrepareCertificates(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	otherCert, _, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, tt := range []struct {
		name      string
		options   CertificateOptions
		stored    map[string]any
		current   map[string]any
		defaults  map[string]any
		installed bool
		reset     bool
		missing   bool
		err       string
	}{
		{name: "existing Secret flag", options: CertificateOptions{CASecret: "operator-ca"}, current: map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}},
		{name: "existing Secret Helm values", current: caReleaseValues("operator-ca", cert)},
		{name: "stored settings preserved", installed: true, stored: caReleaseValues("operator-ca", cert), current: map[string]any{"unrelated": true}},
		{name: "same Secret flag upgrade", installed: true, stored: caReleaseValues("operator-ca", cert), options: CertificateOptions{CASecret: "operator-ca"}},
		{name: "missing stored trust rejected even with flag", installed: true, stored: caReleaseValues("operator-ca", nil), options: CertificateOptions{CASecret: "operator-ca"}, err: "changing trust"},
		{name: "PEM whitespace", installed: true, stored: caReleaseValues("operator-ca", append([]byte("\n "), cert...))},
		{name: "reset rejected", installed: true, stored: caReleaseValues("operator-ca", cert), reset: true, err: "cannot discard stored"},
		{name: "mode switch rejected", installed: true, stored: caReleaseValues("operator-ca", cert), current: map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "selfSigned", "caBundle": nil, "caSecret": nil}}}}, err: "mode changes"},
		{name: "enable CA on installed selfSigned rejected", installed: true, options: CertificateOptions{CASecret: "operator-ca"}, err: "mode changes"},
		{name: "new Secret upgrade rejected", installed: true, stored: caReleaseValues("operator-ca", cert), options: CertificateOptions{CASecret: "another-ca"}, err: "changing the signing Secret"},
		{name: "trust replacement rejected", installed: true, stored: caReleaseValues("operator-ca", cert), current: caReleaseValues("operator-ca", otherCert), err: "changing trust"},
		{name: "Secret rotated out of band rejected", installed: true, stored: caReleaseValues("operator-ca", otherCert), err: "changing trust"},
		{name: "chain as trust rejected", current: caReleaseValues("operator-ca", append(append([]byte{}, cert...), otherCert...)), err: "exactly the dedicated"},
		{name: "CA with authz off rejected", options: CertificateOptions{CASecret: "operator-ca"}, err: "require global.rbac"},
		{name: "flags and values conflict", options: CertificateOptions{CASecret: "operator-ca"}, current: caReleaseValues("operator-ca", cert), err: "cannot be combined"},
		{name: "missing Secret rejected", current: caReleaseValues("operator-ca", cert), missing: true, err: "cannot read signing CA Secret"},
		{name: "unknown mode rejected", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "unknown"}}}}, err: "unsupported certificate mode"},
		{name: "invalid settings shape rejected", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": "invalid"}}}, err: "must be a mapping"},
		{name: "issuer value rejected when off", current: map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "issuerRef"}}}}, err: "require global.rbac"},
		{name: "issuer chart default rejected when off", defaults: map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "issuerRef"}}}}, err: "require global.rbac"},
		{name: "ordinary off install", current: map[string]any{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kube := k8sfake.NewClientset()
			if !tt.missing {
				require.NoError(t, kube.Tracker().Add(caTLSSecret("operator-ca", cert, key)))
			}
			options := NewDefaultClusterOptions()
			options.Certificates, options.RadiusReleaseValues, options.ResetValues = tt.options, tt.stored, tt.reset
			current := copyValues(tt.current)
			storedBefore := copyValues(tt.stored)
			impl := &Impl{newCertificateClient: func(context string) (kubernetes.Interface, error) {
				require.Equal(t, "selected-cluster", context)
				return kube, nil
			}, newRotationStore: emptyRotationTestStore}
			plan, err := impl.prepareCertificates(t.Context(), options, tt.defaults, current, "selected-cluster", tt.installed)
			require.Equal(t, storedBefore, copyValues(tt.stored))
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb(), "preparation must be read only")
			}
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				require.Nil(t, plan)
				return
			}
			require.NoError(t, err)
			if plan != nil {
				require.Nil(t, plan.secret, "operator-owned Secrets are never adopted or changed")
				require.Equal(t, certificateValues(caReleaseValues("operator-ca", cert)), certificateValues(current))
			}
		})
	}
}

func TestRadiusCertificateImportRetainedAfterFailureAndUninstall(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, cert, 0600))
	require.NoError(t, os.WriteFile(keyPath, key, 0600))
	client := NewMockHelmClient(gomock.NewController(t))
	kube := k8sfake.NewClientset()
	impl := &Impl{
		Helm:                 client,
		newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil },
		newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
			k, a := (fakeCertManagerCluster{crd: true, controller: certManagerController("security", "v1.21.2", "")}).clients()
			return k, a, nil
		},
		removeDefaultContourGateway: func(context.Context, string) error { return nil }, newRotationStore: emptyRotationTestStore,
	}
	options := NewDefaultClusterOptions()
	options.Contour.Disabled = true
	options.Radius.ChartPath = "local-chart"
	options.Radius.SetArgs = []string{"global.rbac.dryRun=true"}
	options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
	client.EXPECT().RunHelmGet(gomock.Any(), "cert-manager").Return(nil, driver.ErrReleaseNotFound)
	client.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(nil, driver.ErrReleaseNotFound)
	var name string
	client.EXPECT().RunHelmInstall(gomock.Any(), gomock.Any(), gomock.Any(), "radius", RadiusSystemNamespace, true, gomock.Any(), gomock.Nil()).
		DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, values map[string]any, _, _ string, _ bool, _ time.Duration, _ map[string]string) (*releasev1.Release, error) {
			name = certificateValues(values)["caSecret"].(string)
			secret, err := kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, key, secret.Data[corev1.TLSPrivateKeyKey])
			serialized, err := json.Marshal(values)
			require.NoError(t, err)
			require.NotContains(t, string(serialized), "PRIVATE KEY")
			require.Equal(t, strings.TrimSpace(string(cert)), certificateValues(values)["caBundle"])
			return nil, errors.New("Radius readiness failed")
		})
	values := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
	plan, err := impl.prepareCertificates(t.Context(), options, nil, values, "context", false)
	require.NoError(t, err)
	require.NoError(t, impl.applyCertificatePlan(t.Context(), plan))
	require.ErrorContains(t, NewHelmAction(client).ApplyHelmChart("context", &chart.Chart{}, &helm.Configuration{},
		options.Radius.ChartOptions, values), "Radius readiness failed")
	require.NotEmpty(t, name)
	client.EXPECT().RunHelmUninstall(gomock.Any(), "radius", RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil)
	client.EXPECT().RunHelmUninstall(gomock.Any(), "contour", RadiusSystemNamespace, true).Return(&release.UninstallReleaseResponse{}, nil)
	require.NoError(t, impl.UninstallRadius(t.Context(), options, "context"))
	retained, err := kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), name, metav1.GetOptions{})
	require.NoError(t, err)
	require.Equal(t, key, retained.Data[corev1.TLSPrivateKeyKey])
	for _, action := range kube.Actions() {
		require.NotContains(t, []string{"delete", "update", "patch"}, action.GetVerb())
	}
}

func TestCertificateImportLifecycle(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) { c.NotAfter = time.Now().Add(29 * 24 * time.Hour) }, nil, nil)
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "ca.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, cert, 0600))
	require.NoError(t, os.WriteFile(keyPath, key, 0600))
	for _, scenario := range []string{"success and retry", "create failure", "create race", "namespace failure", "missing key file", "malformed files"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			kube := k8sfake.NewClientset()
			var logs strings.Builder
			impl := &Impl{
				newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil },
				logger:               func(format string, values ...any) { fmt.Fprintf(&logs, format, values...) }, newRotationStore: emptyRotationTestStore,
			}
			options := NewDefaultClusterOptions()
			options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
			switch scenario {
			case "missing key file":
				options.Certificates.CAKey = filepath.Join(dir, "missing")
			case "malformed files":
				options.Certificates.CACert = keyPath
			case "create failure":
				kube.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("private-key-sentinel")
				})
			case "create race":
				kube.PrependReactor("create", "secrets", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Resource: "secrets"}, "raced")
				})
			case "namespace failure":
				kube.PrependReactor("create", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("namespace permissions")
				})
			}
			current := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
			plan, err := impl.prepareCertificates(t.Context(), options, nil, current, "context", false)
			if scenario == "missing key file" || scenario == "malformed files" {
				require.Error(t, err)
				require.Empty(t, kube.Actions())
				return
			}
			require.NoError(t, err)
			require.NotNil(t, plan.secret)
			require.Contains(t, logs.String(), "within 30 days")
			valuesJSON, err := json.Marshal(current)
			require.NoError(t, err)
			require.NotContains(t, string(valuesJSON), "PRIVATE KEY")
			require.NotContains(t, string(valuesJSON), keyPath)
			require.Empty(t, plan.secret.OwnerReferences)
			require.Empty(t, plan.secret.Labels)
			require.Empty(t, plan.secret.Annotations)
			err = impl.applyCertificatePlan(t.Context(), plan)
			if scenario != "success and retry" {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-key-sentinel")
			} else {
				require.NoError(t, err)
				require.Contains(t, logs.String(), "retained on failure and uninstall")
				retryValues := map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}
				retryPlan, err := impl.prepareCertificates(t.Context(), options, nil, retryValues, "context", false)
				require.NoError(t, err)
				require.Nil(t, retryPlan.secret)
				require.Equal(t, current, retryValues)
				require.NoError(t, impl.applyCertificatePlan(t.Context(), retryPlan))
				// A conflicting Secret is rejected without adopting or overwriting it.
				existing, err := kube.CoreV1().Secrets(RadiusSystemNamespace).Get(t.Context(), plan.secret.Name, metav1.GetOptions{})
				require.NoError(t, err)
				existing.Data[corev1.TLSPrivateKeyKey] = []byte("private-key-sentinel")
				require.NoError(t, kube.Tracker().Update(corev1.SchemeGroupVersion.WithResource("secrets"), existing, RadiusSystemNamespace))
				_, err = impl.prepareCertificates(t.Context(), options, nil, map[string]any{"global": map[string]any{"rbac": map[string]any{"dryRun": true}}}, "context", false)
				require.ErrorContains(t, err, "will not be overwritten")
			}
			for _, action := range kube.Actions() {
				require.NotContains(t, []string{"update", "patch", "delete"}, action.GetVerb())
			}
			require.NotContains(t, logs.String(), "PRIVATE KEY")
			require.NotContains(t, logs.String(), "private-key-sentinel")
		})
	}
}

func TestRadiusCertificateHelmValues(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	for _, operation := range []string{"install", "upgrade", "reinstall", "preflight-only", "blocked upgrade", "blocked install"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			helmClient := NewMockHelmClient(gomock.NewController(t))
			kube := k8sfake.NewClientset(caTLSSecret("operator-ca", cert, key))
			impl := &Impl{
				Helm:                 helmClient,
				newCertificateClient: func(string) (kubernetes.Interface, error) { return kube, nil },
				newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) {
					k, a := (fakeCertManagerCluster{crd: true, controller: certManagerController("security", "v1.21.2", "")}).clients()
					return k, a, nil
				}, newRotationStore: emptyRotationTestStore,
			}

			options := NewDefaultClusterOptions()
			options.Contour.Disabled = true
			options.Radius.ChartPath = "local-chart"
			options.Radius.SetArgs = []string{"global.imageTag=new"}
			helmClient.EXPECT().LoadChart("local-chart").Return(&chart.Chart{}, nil)
			if operation == "install" {
				options.Radius.SetArgs = append(options.Radius.SetArgs, "global.rbac.dryRun=true")
				options.Certificates.CASecret = "operator-ca"
			} else {
				options.RadiusReleaseValues = caReleaseValues("operator-ca", cert)
			}
			if operation == "blocked upgrade" || operation == "blocked install" {
				options.Radius.SetArgs = []string{"global.rbac.certificates.mode=selfSigned"}
			} else if operation != "preflight-only" && operation != "install" {
				helmClient.EXPECT().RunHelmGet(gomock.Any(), "cert-manager").Return(nil, driver.ErrReleaseNotFound)
			}
			assertValues := func(values map[string]any) {
				require.Equal(t, certificateValues(caReleaseValues("operator-ca", cert)), certificateValues(values))
				serialized, err := json.Marshal(values)
				require.NoError(t, err)
				require.NotContains(t, string(serialized), "PRIVATE KEY")
				require.Equal(t, "new", nestedValue(values, "global", "imageTag"))
			}
			switch operation {
			case "install":
				// A chart without the bootstrap protocol cannot install enabled
				// Radius by falling back to an unrelated Helm aggregation CA.
			case "reinstall", "upgrade":
				if operation == "reinstall" {
					options.Radius.Reinstall, options.RadiusInstalled = true, true
					helmClient.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(&releasev1.Release{Chart: &chart.Chart{Metadata: &chart.Metadata{Version: "0.1.0"}}}, nil)
				}
				helmClient.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), "radius", RadiusSystemNamespace, true, true, gomock.Any()).
					DoAndReturn(func(_ *helm.Configuration, _ *chart.Chart, values map[string]any, _, _ string, _, _ bool, _ time.Duration) (*releasev1.Release, error) {
						assertValues(values)
						return &releasev1.Release{}, nil
					})
			}
			var err error
			switch operation {
			case "install", "reinstall", "blocked install":
				err = impl.InstallRadius(t.Context(), options, "selected-context")
			case "preflight-only":
				err = impl.CheckRadiusCertificates(t.Context(), options, "selected-context")
			default:
				err = impl.UpgradeRadius(t.Context(), options, "selected-context")
			}
			if strings.HasPrefix(operation, "blocked") {
				require.ErrorContains(t, err, "mode changes")
			} else if operation == "install" {
				require.ErrorContains(t, err, "configured-CA aggregation bootstrap")
			} else {
				require.NoError(t, err)
			}
			for _, action := range kube.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func issuerReleaseValues(ca []byte) map[string]any {
	values := caReleaseValues("", ca)
	settings := certificateValues(values)
	settings["mode"] = "issuerRef"
	settings["issuerRef"] = map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": "external-ca"}
	return values
}

func TestPrepareIssuerCertificates(t *testing.T) {
	t.Parallel()
	cert, key, _ := testSigningCA(t, nil, nil, nil, nil)
	other, _, _ := testSigningCA(t, nil, nil, nil, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "public.pem")
	require.NoError(t, os.WriteFile(path, cert, 0600))
	for _, tt := range []struct {
		name   string
		values bool
		edit   func(*ClusterOptions, map[string]any)
		err    string
	}{
		{name: "fresh explicit values", values: true, edit: func(o *ClusterOptions, v map[string]any) { o.RadiusInstalled = false; o.RadiusReleaseValues = nil }},
		{name: "stored issuer and trust preserved"},
		{name: "stored chart-default issuer fields", edit: func(o *ClusterOptions, v map[string]any) {
			certificateValues(o.RadiusReleaseValues)["issuerRef"] = map[string]any{"name": "external-ca"}
		}},
		{name: "same issuer flag reuses stored bundle", edit: func(o *ClusterOptions, v map[string]any) { o.Certificates.CertIssuer = "Issuer/external-ca" }},
		{name: "same issuer and bundle flags", edit: func(o *ClusterOptions, v map[string]any) {
			o.Certificates = CertificateOptions{CertIssuer: "Issuer/external-ca", CABundle: path}
		}},
		{name: "fresh flag requires public bundle", edit: func(o *ClusterOptions, v map[string]any) {
			o.RadiusInstalled, o.RadiusReleaseValues = false, nil
			o.Certificates.CertIssuer = "Issuer/external-ca"
		}, err: "requires --ca-bundle"},
		{name: "missing bundle file", edit: func(o *ClusterOptions, v map[string]any) {
			o.Certificates = CertificateOptions{CertIssuer: "Issuer/external-ca", CABundle: filepath.Join(dir, "missing")}
		}, err: "cannot read --ca-bundle"},
		{name: "issuer switch rejected", edit: func(o *ClusterOptions, v map[string]any) { o.Certificates.CertIssuer = "Issuer/another-ca" }, err: "changing the certificate issuer"},
		{name: "mode switch rejected", values: true, edit: func(o *ClusterOptions, v map[string]any) { certificateValues(v)["mode"] = "selfSigned" }, err: "mode changes"},
		{name: "trust replacement rejected", values: true, edit: func(o *ClusterOptions, v map[string]any) { certificateValues(v)["caBundle"] = string(other) }, err: "coordinated CA migration"},
		{name: "stored trust missing", edit: func(o *ClusterOptions, v map[string]any) {
			delete(certificateValues(o.RadiusReleaseValues), "caBundle")
		}, err: "caBundle"},
		{name: "chain prohibited", values: true, edit: func(o *ClusterOptions, v map[string]any) {
			certificateValues(v)["caBundle"] = string(append(append([]byte{}, cert...), cert...))
		}, err: "exactly one dedicated"},
		{name: "private key prohibited", values: true, edit: func(o *ClusterOptions, v map[string]any) { certificateValues(v)["caBundle"] = string(key) }, err: "only PEM certificates"},
		{name: "wrong issuer group", values: true, edit: func(o *ClusterOptions, v map[string]any) {
			certificateValues(v)["issuerRef"] = map[string]any{"group": "other.io", "kind": "Issuer", "name": "external-ca"}
		}, err: "group cert-manager.io"},
		{name: "invalid issuer map", values: true, edit: func(o *ClusterOptions, v map[string]any) { certificateValues(v)["issuerRef"] = "external-ca" }, err: "requires group"},
		{name: "signing Secret prohibited", values: true, edit: func(o *ClusterOptions, v map[string]any) { certificateValues(v)["caSecret"] = "operator-ca" }, err: "must not specify a signing Secret"},
		{name: "reset values rejected", edit: func(o *ClusterOptions, v map[string]any) { o.ResetValues = true }, err: "cannot discard stored"},
		{name: "new prerequisite references allowed", edit: func(o *ClusterOptions, v map[string]any) {
			o.Certificates.CertIssuer = "Issuer/external-ca"
			setCertificateValues(v, map[string]any{"requestPolicy": map[string]any{"certManager": map[string]any{"namespace": "security", "deployment": "cm"}}})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			options := NewDefaultClusterOptions()
			options.RadiusInstalled = true
			options.RadiusReleaseValues = issuerReleaseValues(cert)
			current := map[string]any{}
			if tt.values {
				current = issuerReleaseValues(cert)
			}
			if tt.edit != nil {
				tt.edit(&options, current)
			}
			before := copyValues(options.RadiusReleaseValues)
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) {
				t.Fatal("external issuer mode must never read a private signing Secret")
				return nil, nil
			}}
			plan, err := impl.prepareCertificates(t.Context(), options, nil, current, "context", options.RadiusInstalled)
			require.Equal(t, before, copyValues(options.RadiusReleaseValues))
			require.Nil(t, plan, "issuer prerequisites are operator-owned")
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(string(cert)), certificateValues(current)["caBundle"])
			require.Equal(t, "external-ca", certificateValues(current)["issuerRef"].(map[string]any)["name"])
			if tt.name == "new prerequisite references allowed" {
				require.Equal(t, "security", nestedValue(certificateValues(current), "requestPolicy", "certManager", "namespace"))
			}
		})
	}
}

func TestRadiusIssuerLivePreflight(t *testing.T) {
	for _, scenario := range []struct {
		name, operation, err string
		clusterIssuer        bool
		intermediate         bool
		explicitContext      bool
		external             bool
		edit                 func(*testing.T, *issuerPolicyFixture)
	}{
		{name: "Issuer install isolated config", operation: "install"},
		{name: "ClusterIssuer install merged explicit context", operation: "install", clusterIssuer: true, explicitContext: true},
		{name: "dedicated intermediate without key or root", operation: "install", intermediate: true},
		{name: "upgrade omitted flags", operation: "upgrade", explicitContext: true},
		{name: "upgrade with stored chart-default issuer fields", operation: "default issuer upgrade"},
		{name: "installed chart issuer default cannot switch silently", operation: "changed default issuer upgrade", err: "changing issuers requires coordinated CA migration"},
		{name: "retry after failed upgrade", operation: "failed upgrade"},
		{name: "pending upgrade rejected", operation: "pending upgrade", err: "live issuer-policy verification failed"},
		{name: "reinstall omitted flags", operation: "reinstall"},
		{name: "preflight-only", operation: "preflight-only"},
		{name: "external aggregation-only", operation: "install", external: true},
		{name: "external ClusterIssuer upgrade", operation: "upgrade", external: true, clusterIssuer: true},
		{name: "missing preprovisioned namespace", operation: "install", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("Namespace", RadiusSystemNamespace).SetName("absent")
		}},
		{name: "missing policy blocks install", operation: "install", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("CertificateRequestPolicy", "radius-radius-system-controller").SetName("missing")
		}},
		{name: "missing policy blocks upgrade", operation: "upgrade", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("CertificateRequestPolicy", "radius-radius-system-controller").SetName("missing")
		}},
		{name: "invalid webhook blocks preflight", operation: "preflight-only", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ValidatingWebhookConfiguration", "cert-manager-webhook").Object["webhooks"].([]any)[0].(map[string]any)["failurePolicy"] = "Ignore"
		}},
		{name: "enabled default approver blocks reinstall", operation: "reinstall", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.container("Deployment", "cert-manager")["args"] = []any{"--controllers=*"}
		}},
		{name: "forbidden policy inventory", operation: "install", err: "forbidden", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.forbiddenPath = "/apis/policy.cert-manager.io/v1alpha1/certificaterequestpolicies"
		}},
		{name: "missing policy-use RBAC", operation: "install", err: "live issuer-policy verification failed", edit: func(t *testing.T, f *issuerPolicyFixture) {
			f.object("ClusterRole", "radius-policy-use").Object["rules"] = []any{}
		}},
		{name: "chart without verifier", operation: "old chart", err: "no shared live issuer-policy verifier"},
		{name: "concurrent stored settings change", operation: "concurrent upgrade", err: "changed since preflight"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newIssuerPolicyFixture(t)
			if scenario.clusterIssuer {
				fixture.useClusterIssuer(t)
			}
			if scenario.external {
				fixture.values["global"].(map[string]any)["rbac"].(map[string]any)["mtls"] = map[string]any{"provider": "external"}
				fixture.values["de"] = externalProviderValues()["de"]
				onlyUCP := fixture.objects[:0]
				for _, object := range fixture.objects {
					if object.GetKind() != "CertificateRequestPolicy" || object.GetName() == "radius-radius-system-ucp" {
						onlyUCP = append(onlyUCP, object)
					}
				}
				fixture.objects = onlyUCP
			}
			if scenario.edit != nil {
				scenario.edit(t, fixture)
			}
			install := scenario.operation == "install" || scenario.operation == "old chart"
			if !install {
				fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
					"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
					"metadata": map[string]any{"name": radiusAPIService, "uid": "existing-aggregation", "resourceVersion": "1"},
				}})
			}
			target := fixture.server(t)
			decoy := k8sutil.NewForbiddenAPIServer(t)
			kubeContext := useTargetKubeconfig(t, target.URL, decoy.URL, scenario.explicitContext)
			ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
			if scenario.intermediate {
				parentKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
				require.NoError(t, err)
				_, _, parent := testSigningCA(t, parentKey, nil, nil, nil)
				ca, _, _ = testSigningCA(t, nil, func(c *x509.Certificate) {
					c.Subject.CommonName = "dedicated-radius-intermediate"
				}, parent, parentKey)
			}
			settings := certificateValues(fixture.values)
			settings["caBundle"] = string(ca)
			options := NewDefaultClusterOptions()
			options.Contour.Disabled, options.Radius.ChartPath = true, "local-chart"
			if install {
				bundlePath := filepath.Join(t.TempDir(), "public-ca.pem")
				require.NoError(t, os.WriteFile(bundlePath, ca, 0600))
				options.Certificates = CertificateOptions{CertIssuer: settings["issuerRef"].(map[string]any)["kind"].(string) + "/external-ca", CABundle: bundlePath}
				options.Radius.SetArgs = []string{"global.rbac.enabled=true"}
				if scenario.external {
					options.Radius.SetArgs = append(options.Radius.SetArgs, "global.rbac.mtls.provider=external",
						"de.externalIdentity.enabled=true", "de.image=local-engine", "de.tag=local-test")
				}
			} else {
				options.RadiusReleaseValues = copyValues(fixture.values)
				if scenario.operation == "default issuer upgrade" || scenario.operation == "changed default issuer upgrade" {
					certificateValues(options.RadiusReleaseValues)["issuerRef"] = map[string]any{"name": "external-ca"}
				}
				options.RadiusInstalled = true
				options.Radius.Reinstall = scenario.operation == "reinstall"
			}
			selected, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
			require.NoError(t, err)
			defaultsBefore := copyValues(selected.Values)
			if scenario.operation == "old chart" {
				selected.Templates = nil
			}
			client := NewMockHelmClient(gomock.NewController(t))
			client.EXPECT().LoadChart("local-chart").Return(selected, nil)
			if !install {
				config := copyValues(options.RadiusReleaseValues)
				installedChart := selected
				if scenario.operation == "changed default issuer upgrade" {
					installedChart = cloneChart(selected)
					installedChart.Values = copyValues(selected.Values)
					certificateValues(installedChart.Values)["issuerRef"].(map[string]any)["kind"] = "ClusterIssuer"
				}
				if scenario.operation == "concurrent upgrade" {
					config["changed"] = true
				}
				history := []*releasev1.Release{{
					Name: "radius", Namespace: RadiusSystemNamespace, Version: 1, Config: config,
					Chart: installedChart, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
				}}
				if scenario.operation == "failed upgrade" || scenario.operation == "pending upgrade" {
					status := releasecommon.StatusFailed
					if scenario.operation == "pending upgrade" {
						status = releasecommon.StatusPendingUpgrade
					}
					history = append(history, &releasev1.Release{
						Name: "radius", Namespace: RadiusSystemNamespace, Version: 2, Config: map[string]any{},
						Chart: selected, Info: &releasev1.Info{Status: status},
					})
				}
				client.EXPECT().RunHelmHistory(gomock.Any(), "radius").Return(history, nil)
			}
			if scenario.err == "" && scenario.operation != "preflight-only" && !install {
				if scenario.operation == "reinstall" {
					client.EXPECT().RunHelmGet(gomock.Any(), "radius").Return(&releasev1.Release{Chart: selected}, nil)
				}
				client.EXPECT().RunHelmUpgrade(gomock.Any(), gomock.Any(), gomock.Any(), "radius", RadiusSystemNamespace, true, true, gomock.Any()).Return(&releasev1.Release{}, nil)
			}
			impl := &Impl{Helm: client}
			switch scenario.operation {
			case "install":
				// This fixture proves the actual read-only policy seam. Fresh
				// lifecycle success additionally requires bootstrap issuance and
				// running-process evidence, not a mocked Helm install return.
				selected, configuration, current, prepareErr := prepareRadiusChart(t.Context(), NewHelmAction(client), options.Radius, kubeContext)
				require.NoError(t, prepareErr)
				_, err = impl.prepareCertificates(t.Context(), options, selected.Values, current, kubeContext, false)
				if err == nil {
					err = impl.checkIssuerPolicy(t.Context(), options, selected, configuration, current, kubeContext, false)
				}
			case "old chart", "reinstall":
				err = impl.InstallRadius(t.Context(), options, kubeContext)
			case "preflight-only":
				err = impl.CheckRadiusCertificates(t.Context(), options, kubeContext)
			default:
				err = impl.UpgradeRadius(t.Context(), options, kubeContext)
			}
			if scenario.err != "" {
				require.ErrorContains(t, err, scenario.err)
			} else {
				require.NoError(t, err)
			}
			require.Empty(t, decoy.Requests(), "must not use another kubeconfig or context")
			require.Equal(t, defaultsBefore, selected.Values, "preflight must not rewrite chart defaults")
		})
	}
}
