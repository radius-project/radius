//go:build !windows

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
	"bytes"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// TestUCPAPIServerPublicSourceProjection_APIServer checks cainjector's projection
// of an explicitly staged public CA, not automatic lifecycle or serving readiness.
//
// It needs cert-manager's cainjector binary, built from the cert-manager release Radius installs, for example:
//
//	git clone --depth 1 --branch v1.21.2 https://github.com/cert-manager/cert-manager
//	(cd cert-manager/cmd/cainjector && go build -o /tmp/cainjector .)
//	CERT_MANAGER_CAINJECTOR=/tmp/cainjector go test ./pkg/cli/helm -run TestUCPAPIServerPublicSourceProjection_APIServer
func TestUCPAPIServerPublicSourceProjection_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	cainjector := os.Getenv("CERT_MANAGER_CAINJECTOR")
	if cainjector == "" {
		t.Skip("Set CERT_MANAGER_CAINJECTOR to a cert-manager cainjector binary to run the CA renewal regression.")
	}
	helmBinary, err := exec.LookPath("helm")
	require.NoError(t, err, "the test renders the chart with the helm binary")

	kubeconfig, client := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	dynamicClient, err := dynamic.NewForConfig(config)
	require.NoError(t, err)
	ctx := t.Context()
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: RadiusSystemNamespace}}, metav1.CreateOptions{})
	require.NoError(t, err)

	_, apiService := renderUCPAPIServerTrustObjects(t, helmBinary)
	_, err = dynamicClient.Resource(apiServicesResource).Create(ctx, apiService, metav1.CreateOptions{})
	require.NoError(t, err)

	// The first CA certificate expires soon, so the test can check trust after it expires.
	issued := reissueTestCA(t, newTestCA(t, "radius-apiserver-ca"), time.Now().Add(40*time.Second))
	source := permanentPublicTrustSecret(issued.pem)
	source.UID, source.ResourceVersion = "", ""
	_, err = client.CoreV1().Secrets(RadiusSystemNamespace).Create(ctx, source, metav1.CreateOptions{})
	require.NoError(t, err)

	startCAInjector(t, cainjector, kubeconfig)

	requireAPIServiceCABundle(t, dynamicClient, issued)

	// The fixture stages public trust directly. This is not lifecycle authorization.
	renewed := reissueTestCA(t, issued, time.Now().Add(2*time.Hour))
	secret, err := client.CoreV1().Secrets(RadiusSystemNamespace).Get(ctx, ucpAPIServerTrustSecretName, metav1.GetOptions{})
	require.NoError(t, err)
	secret.Data[caCertificateKey] = renewed.pem
	_, err = client.CoreV1().Secrets(RadiusSystemNamespace).Update(ctx, secret, metav1.UpdateOptions{})
	require.NoError(t, err)

	bundle := requireAPIServiceCABundle(t, dynamicClient, renewed)

	// The API server verifies UCP's certificate against the bundle after the first CA certificate expired.
	leaf := mustParseCertificate(t, issueUCPCertificate(t, renewed))
	time.Sleep(time.Until(issued.cert.NotAfter.Add(time.Second)))
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(bundle))
	_, err = leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "ucp." + RadiusSystemNamespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.NoError(t, err)
	onlyFirst := x509.NewCertPool()
	onlyFirst.AddCert(issued.cert)
	_, err = leaf.Verify(x509.VerifyOptions{Roots: onlyFirst, DNSName: "ucp." + RadiusSystemNamespace + ".svc", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	require.Error(t, err, "a bundle that kept only the expired CA certificate must not verify UCP")
}

// renderUCPAPIServerTrustObjects renders the radius-apiserver-ca Certificate and the UCP APIService with
// authorization on, as a fresh install renders them.
func renderUCPAPIServerTrustObjects(t *testing.T, helmBinary string) (*unstructured.Unstructured, *unstructured.Unstructured) {
	t.Helper()
	output, err := exec.CommandContext(t.Context(), helmBinary, "template", "radius", filepath.Join("..", "..", "..", "deploy", "Chart"),
		"--namespace", RadiusSystemNamespace, "--set", "global.rbac.enabled=true",
		"--show-only", "templates/global/certificates.yaml", "--show-only", "templates/ucp/apiservice.yaml").CombinedOutput()
	require.NoError(t, err, string(output))

	var caCertificate, apiService *unstructured.Unstructured
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(output), 4096)
	for {
		object := &unstructured.Unstructured{}
		err := decoder.Decode(&object.Object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch {
		case object.GetKind() == "Certificate" && object.GetName() == ucpAPIServerCASecretName:
			caCertificate = object
		case object.GetKind() == "APIService":
			apiService = object
		}
	}
	require.NotNil(t, caCertificate, "the chart must render the radius-apiserver-ca Certificate")
	require.NotNil(t, apiService, "the chart must render the UCP APIService")
	return caCertificate, apiService
}

// startCAInjector runs cert-manager's cainjector against the test API server until the test ends.
func startCAInjector(t *testing.T, binary, kubeconfig string) {
	t.Helper()
	var logs bytes.Buffer
	cmd := exec.Command(binary, "--kubeconfig", kubeconfig, "--leader-elect=false", "--enable-certificates-data-source=false",
		"--metrics-listen-address=0")
	// startTestAPIServer points KUBECONFIG at an unusable file, which cainjector would otherwise prefer.
	cmd.Env = append(os.Environ(), "KUBECONFIG="+kubeconfig)
	cmd.Stdout, cmd.Stderr = &logs, &logs
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("cainjector logs:\n%s", logs.String())
		}
	})
}

// requireAPIServiceCABundle waits until cainjector projects the permanent public source and the
// bundle holds want, and returns the bundle. cainjector keeps earlier certificates until they expire, so the bundle
// may also hold earlier certificates of the same CA, but nothing else.
func requireAPIServiceCABundle(t *testing.T, client dynamic.Interface, want testCA) []byte {
	t.Helper()
	var bundle []byte
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		apiService, err := client.Resource(apiServicesResource).Get(t.Context(), ucpAPIServiceName, metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		assert.Equal(c, ucpAPIServerCASource(RadiusSystemNamespace), apiService.GetAnnotations()[injectCAFromSecretAnnotation])
		encoded, _, _ := unstructured.NestedString(apiService.Object, "spec", "caBundle")
		bundle, err = base64.StdEncoding.DecodeString(encoded)
		if !assert.NoError(c, err) {
			return
		}
		certs, err := parseCertificateBundlePEM(bundle)
		if !assert.NoError(c, err) {
			return
		}
		found := false
		for _, cert := range certs {
			assert.True(c, cert.IsCA, "the caBundle holds a certificate that is not a CA")
			assert.True(c, want.key.PublicKey.Equal(cert.PublicKey), "the caBundle holds a certificate of another CA: %s", cert.Subject)
			found = found || bytes.Equal(cert.Raw, want.cert.Raw)
		}
		assert.True(c, found, "the caBundle does not hold the current radius-apiserver-ca certificate")
	}, 30*time.Second, 200*time.Millisecond)
	return bundle
}

// reissueTestCA reissues ca with the same key and subject, a new serial and notAfter, as cert-manager renews a CA
// with rotationPolicy: Never.
func reissueTestCA(t *testing.T, ca testCA, notAfter time.Time) testCA {
	t.Helper()
	template := *ca.cert
	template.SerialNumber = big.NewInt(time.Now().UnixNano())
	template.NotBefore, template.NotAfter = time.Now().Add(-time.Minute), notAfter
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &ca.key.PublicKey, ca.key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	return testCA{cert: cert, key: ca.key, pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func mustParseCertificate(t *testing.T, pemBytes []byte) *x509.Certificate {
	t.Helper()
	cert, err := firstCertificate(pemBytes)
	require.NoError(t, err)
	return cert
}
