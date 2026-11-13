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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/radius-project/radius/test/k8sutil"
	"github.com/stretchr/testify/require"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	releasev1 "helm.sh/helm/v4/pkg/release/v1"
	corev1 "k8s.io/api/core/v1"
	apiextclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func certificateTestChart(t *testing.T) *chart.Chart {
	t.Helper()
	c, err := loader.Load(filepath.Join("..", "..", "..", "deploy", "Chart"))
	require.NoError(t, err)
	return c
}

func TestCertificateTargetProfiles(t *testing.T) {
	for _, tt := range []struct {
		name, provider, domain string
		de                     bool
		emptyDomain            bool
		edit                   func(*x509.Certificate)
		fail                   bool
	}{
		{name: "both profiles constrained", provider: "radius"},
		{name: "empty domain uses chart fallback", provider: "radius", emptyDomain: true},
		{name: "external only DNS", provider: "external", edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "URI exclusion", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.ExcludedURIDomains = []string{"radius.internal"} }},
		{name: "URI wrong domain", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"other.example"} }},
		{name: "DNS wrong namespace", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.PermittedDNSDomains = []string{"ucp.other.svc"} }},
		{name: "external DNS exclusion", provider: "external", fail: true, edit: func(c *x509.Certificate) { c.ExcludedDNSDomains = []string{"ucp.radius-system.svc"} }},
		{name: "custom domain with DE receiver", provider: "radius", domain: "custom.example", de: true},
		{name: "custom domain incompatible", provider: "radius", domain: "custom.example", fail: true, edit: func(c *x509.Certificate) { c.PermittedURIDomains = []string{"radius.internal"} }},
		{name: "CSI requires client usage despite token caller", provider: "radius", fail: true, edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }},
		{name: "external server usage only", provider: "external", edit: func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth} }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			domain := tt.domain
			if domain == "" {
				domain = "radius.internal"
			}
			ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
				c.MaxPathLenZero = true
				c.PermittedDNSDomainsCritical = true
				c.PermittedDNSDomains = []string{"ucp.radius-system.svc"}
				c.PermittedURIDomains = []string{domain}
				if tt.edit != nil {
					tt.edit(c)
				}
			}, nil, nil)
			client := fake.NewClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "operator-ca", Namespace: RadiusSystemNamespace, UID: "ca-uid", ResourceVersion: "17"},
				Type:       corev1.SecretTypeTLS, Data: map[string][]byte{corev1.TLSCertKey: ca, corev1.TLSPrivateKeyKey: key},
			})
			options := NewDefaultClusterOptions()
			current := caReleaseValues("operator-ca", ca)
			rbac := current["global"].(map[string]any)["rbac"].(map[string]any)
			rbac["tls"] = map[string]any{"provider": tt.provider}
			rbac["spiffe"] = map[string]any{"trustDomain": domain}
			if tt.emptyDomain {
				rbac["spiffe"] = map[string]any{"trustDomain": ""}
			}
			current["de"] = map[string]any{"internalAuth": map[string]any{"enabled": tt.de}}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
			if tt.fail {
				require.ErrorContains(t, err, "cannot support the selected URI/DNS")
				require.Nil(t, target)
			} else {
				require.Equal(t, domain, target.TrustDomain)
				require.NoError(t, err)
				require.Nil(t, plan.secret)
				require.Equal(t, "ca-uid", string(target.SigningUID))
				require.Equal(t, "17", target.SigningVersion)
				require.Equal(t, RadiusAPIServerIssuerName, target.Planes["aggregation"].Issuer.Name)
				require.NotEmpty(t, target.Planes["aggregation"].CA)
				if tt.provider == "external" {
					require.Len(t, target.Planes, 1)
					require.Empty(t, target.Receivers)
				} else {
					require.Equal(t, RadiusCAIssuerName, target.Planes["internal"].Issuer.Name)
					require.Contains(t, target.Receivers, "ucp")
					require.NotContains(t, target.Receivers, "controller")
					require.Equal(t, tt.de, slices.Contains(target.Receivers, "bicep-de"))
				}
				public, err := json.Marshal(target)
				require.NoError(t, err)
				require.NotContains(t, string(public), "PRIVATE KEY")
			}
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestCertificateTargetPolicyRenderer(t *testing.T) {
	fixture := newIssuerPolicyFixture(t)
	fixture.objects = append(fixture.objects, &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1", "kind": "Issuer",
		"metadata": map[string]any{"name": RadiusCAIssuerName, "namespace": RadiusSystemNamespace},
		"spec":     map[string]any{"ca": map[string]any{"secretName": "radius-ca"}},
	}})
	server := fixture.server(t)
	decoy := k8sutil.NewForbiddenAPIServer(t)
	kubeContext := useTargetKubeconfig(t, server.URL, decoy.URL, false)
	ca, _, _ := testSigningCA(t, nil, nil, nil, nil)
	certificateValues(fixture.values)["caBundle"] = string(ca)
	options := NewDefaultClusterOptions()
	options.RadiusInstalled = true
	options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	selected := certificateTestChart(t)
	history := []*releasev1.Release{{
		Name: "radius", Namespace: RadiusSystemNamespace, Version: 1, Config: copyValues(options.RadiusReleaseValues),
		Chart: selected, Info: &releasev1.Info{Status: releasecommon.StatusDeployed},
	}}
	impl := &Impl{}
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, fixture.values), "mode changes")
	require.ErrorContains(t, impl.renderCertificatePolicy(t.Context(), options, selected, fixture.values, kubeContext, history), "certificate lifecycle prerequisite")
	require.NoError(t, impl.renderCertificatePolicy(t.Context(), options, certificatePolicyChart(selected), fixture.values, kubeContext, history))
	history[0].Config["changed"] = true
	require.ErrorContains(t, impl.renderCertificatePolicy(t.Context(), options, certificatePolicyChart(selected), fixture.values, kubeContext, history), "changed since preflight")
	require.Empty(t, decoy.Requests())
}

func TestCertificateTargetParentConstraints(t *testing.T) {
	parentKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	parentPEM, _, parent := testSigningCA(t, parentKey, func(c *x509.Certificate) {
		c.PermittedURIDomains = []string{"wrong.example"}
		c.PermittedDNSDomainsCritical = true
	}, nil, nil)
	ca, key, _ := testSigningCA(t, nil, func(c *x509.Certificate) {
		c.Subject.CommonName = "dedicated-intermediate"
	}, parent, parentKey)
	chain := append(append([]byte{}, ca...), parentPEM...)
	for _, source := range []string{"stored values", "Secret flag", "file import"} {
		t.Run(source, func(t *testing.T) {
			options := NewDefaultClusterOptions()
			current := caReleaseValues("operator-ca", ca)
			client := fake.NewClientset(caTLSSecret("operator-ca", chain, key))
			if source != "stored values" {
				current = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
				options.Certificates.CASecret = "operator-ca"
			}
			if source == "file import" {
				directory := t.TempDir()
				certPath, keyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "key.pem")
				require.NoError(t, os.WriteFile(certPath, chain, 0600))
				require.NoError(t, os.WriteFile(keyPath, key, 0600))
				options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
				client = fake.NewClientset()
			}
			impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
			target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
			require.ErrorContains(t, err, "CA certificate 2 cannot support")
			require.Nil(t, target)
			require.Nil(t, plan)
			for _, action := range client.Actions() {
				require.Equal(t, "get", action.GetVerb())
			}
		})
	}
}

func TestCertificateTargetTransitionAndLease(t *testing.T) {
	ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
	directory := t.TempDir()
	certPath, keyPath := filepath.Join(directory, "ca.pem"), filepath.Join(directory, "key.pem")
	require.NoError(t, os.WriteFile(certPath, ca, 0600))
	require.NoError(t, os.WriteFile(keyPath, key, 0600))
	options := NewDefaultClusterOptions()
	options.RadiusInstalled = true
	options.RadiusReleaseValues = map[string]any{"global": map[string]any{"rbac": map[string]any{"enabled": true}}}
	options.Certificates = CertificateOptions{CACert: certPath, CAKey: keyPath}
	client := fake.NewClientset()
	impl := &Impl{newCertificateClient: func(string) (kubernetes.Interface, error) { return client, nil }}
	target, plan, err := impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), map[string]any{}, "context")
	require.NoError(t, err)
	require.Equal(t, "caSecret", target.Mode)
	require.NotNil(t, plan.secret)
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, target.Values), "mode changes")
	require.ErrorContains(t, impl.applyCertificatePlan(t.Context(), plan), "Lease write guard")
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	plan.beforeWrite = func(context.Context) error { return errors.New("lease lost") }
	require.ErrorContains(t, impl.applyCertificatePlan(t.Context(), plan), "lease lost")
	for _, action := range client.Actions() {
		require.Equal(t, "get", action.GetVerb())
	}
	options.Certificates = CertificateOptions{}
	options.RadiusReleaseValues = caReleaseValues("previous-ca", ca)
	current := map[string]any{"global": map[string]any{"rbac": map[string]any{"certificates": map[string]any{"mode": "selfSigned"}}}}
	target, plan, err = impl.prepareCertificateTarget(t.Context(), options, certificateTestChart(t), current, "context")
	require.NoError(t, err)
	require.Nil(t, plan)
	require.Equal(t, "selfSigned", target.Mode)
	require.True(t, target.Planes["internal"].Unresolved)
	require.True(t, target.Planes["aggregation"].Unresolved)
	require.Empty(t, certificateValues(target.Values)["caBundle"])
	require.ErrorContains(t, unchangedCertificateSelection(options.RadiusReleaseValues, current), "mode changes")
}

func TestCASecretLivePreflight(t *testing.T) {
	for _, provider := range []string{"radius", "external"} {
		for _, invalidInjector := range []bool{false, true} {
			t.Run(provider+fmt.Sprint("/invalidInjector=", invalidInjector), func(t *testing.T) {
				ca, key, _ := testSigningCA(t, nil, nil, nil, nil)
				fixture := newIssuerPolicyFixture(t)
				rbac := fixture.values["global"].(map[string]any)["rbac"].(map[string]any)
				rbac["tls"] = map[string]any{"provider": provider}
				rbac["certificates"] = map[string]any{"mode": "caSecret", "caSecret": "operator-ca", "caBundle": string(ca)}
				server := fixture.server(t)
				decoy := k8sutil.NewForbiddenAPIServer(t)
				kubeContext := useTargetKubeconfig(t, server.URL, decoy.URL, false)
				d, rs, pod := injectorFixture("cm-cainjector")
				if invalidInjector {
					d.Spec.Template.Spec.Containers[0].Args = []string{"--config=/opaque.yaml"}
					pod.Spec = *d.Spec.Template.Spec.DeepCopy()
				}
				controller := certManagerController("security", "v1.21.2", "")
				client := fake.NewClientset(d, rs, pod, controller, caTLSSecret("operator-ca", ca, key))
				_, extensions := (fakeCertManagerCluster{crd: true}).clients()
				impl := &Impl{
					newCertificateClient:  func(string) (kubernetes.Interface, error) { return client, nil },
					newCertManagerClients: func(string) (kubernetes.Interface, apiextclient.Interface, error) { return client, extensions, nil },
				}
				options := NewDefaultClusterOptions()
				current := copyValues(fixture.values)
				selected := certificateTestChart(t)
				_, _, err := impl.prepareSelectedCertificates(t.Context(), options, selected, current, kubeContext, false)
				require.NoError(t, err)
				err = impl.checkIssuerPolicy(t.Context(), options, selected, nil, current, kubeContext, false)
				if invalidInjector {
					require.ErrorContains(t, err, "not an inspectable")
				} else {
					require.NoError(t, err)
				}
				for _, action := range client.Actions() {
					require.Contains(t, []string{"get", "list"}, action.GetVerb())
				}
				require.Empty(t, decoy.Requests())
			})
		}
	}
}
