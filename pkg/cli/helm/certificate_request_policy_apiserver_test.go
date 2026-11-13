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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var certificateRequestsResource = schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificaterequests"}

// TestCertificateRequestIssuancePolicy_APIServer renders the chart's radius-certificate-issuance policy and checks,
// against a real API server's CEL admission, which requesters may create CertificateRequests for which issuers.
func TestCertificateRequestIssuancePolicy_APIServer(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("Run make test or set KUBEBUILDER_ASSETS to run the isolated API-server regression.")
	}
	helmBinary, err := exec.LookPath("helm")
	require.NoError(t, err, "the policy test renders the chart with the helm binary")

	const certManagerUser = "system:serviceaccount:security:cert-manager"
	kubeconfig, client := startTestAPIServer(t)
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	require.NoError(t, err)
	ctx := t.Context()

	crdClient, err := apiextensionsclient.NewForConfig(config)
	require.NoError(t, err)
	createCertificateRequestCRD(t, crdClient)
	for _, namespace := range []string{RadiusSystemNamespace, "other"} {
		_, err := client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	// Every requester below may create CertificateRequests, so only the policy can deny them.
	_, err = client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "create-certificaterequests"},
		Rules:      []rbacv1.PolicyRule{{APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificaterequests"}, Verbs: []string{"create"}}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "create-certificaterequests"},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "create-certificaterequests"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", APIGroup: "rbac.authorization.k8s.io", Name: "system:authenticated"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	createRenderedIssuancePolicy(t, helmBinary, config, client, "global.rbac.dryRun=true", "global.rbac.certManager.requesters={"+certManagerUser+"}")

	radiusSA := func(name string) string { return "system:serviceaccount:" + RadiusSystemNamespace + ":" + name }
	issuer := func(name string) map[string]any {
		return map[string]any{"group": "cert-manager.io", "kind": "Issuer", "name": name}
	}
	tests := []struct {
		name      string
		user      string
		namespace string
		issuerRef map[string]any
		wantRule  string
	}{
		{name: "component requests a legacy mTLS certificate", user: radiusSA("applications-rp"), issuerRef: issuer(RadiusMTLSIssuerName), wantRule: "reserved-issuer"},
		{name: "UCP requests from the mTLS issuer", user: radiusSA("ucp"), issuerRef: issuer(RadiusMTLSIssuerName), wantRule: "reserved-issuer"},
		{name: "component requests a CA from the bootstrap issuer", user: radiusSA("dynamic-rp"), issuerRef: issuer(RadiusBootstrapIssuerName), wantRule: "reserved-issuer"},
		{name: "default kind and group still name the mTLS issuer", user: radiusSA("ucp"), issuerRef: map[string]any{"name": RadiusMTLSIssuerName}, wantRule: "reserved-issuer"},
		{name: "another namespace's service account requests from the mTLS issuer", user: "system:serviceaccount:other:app", issuerRef: issuer(RadiusMTLSIssuerName), wantRule: "reserved-issuer"},
		{name: "a user requests from the bootstrap issuer", user: "alice", issuerRef: issuer(RadiusBootstrapIssuerName), wantRule: "reserved-issuer"},
		{name: "component requests from a ClusterIssuer", user: radiusSA("applications-rp"), issuerRef: map[string]any{"group": "cert-manager.io", "kind": "ClusterIssuer", "name": RadiusMTLSIssuerName}, wantRule: "radius-ca-issuer-only"},
		{name: "component requests from another issuer type", user: radiusSA("ucp"), issuerRef: map[string]any{"group": "example.com", "kind": "Issuer", "name": RadiusCAIssuerName}, wantRule: "radius-ca-issuer-only"},
		{name: "csi-driver-spiffe requests a SPIFFE certificate as the pod's service account", user: radiusSA("ucp"), issuerRef: issuer(RadiusCAIssuerName)},
		{name: "cert-manager issues a component mTLS certificate", user: certManagerUser, issuerRef: issuer(RadiusMTLSIssuerName)},
		{name: "cert-manager renews a component mTLS certificate with a new request", user: certManagerUser, issuerRef: issuer(RadiusMTLSIssuerName)},
		{name: "cert-manager issues the Radius CAs", user: certManagerUser, issuerRef: issuer(RadiusBootstrapIssuerName)},
		{name: "requests in other namespaces are not checked", user: radiusSA("ucp"), namespace: "other", issuerRef: issuer(RadiusMTLSIssuerName)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := tt.namespace
			if namespace == "" {
				namespace = RadiusSystemNamespace
			}
			err := createCertificateRequestAs(t, config, tt.user, namespace, tt.issuerRef)
			if tt.wantRule == "" {
				require.NoError(t, err)
				return
			}
			require.True(t, apierrors.IsInvalid(err) || apierrors.IsForbidden(err), "got %v", err)
			require.ErrorContains(t, err, "CertificateRequestDenied: "+tt.wantRule)
			require.ErrorContains(t, err, tt.user)
		})
	}
}

func createCertificateRequestCRD(t *testing.T, crdClient apiextensionsclient.Interface) {
	t.Helper()

	preserve := true
	_, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Create(t.Context(), &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "certificaterequests.cert-manager.io"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "cert-manager.io",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: "certificaterequests", Singular: "certificaterequest", Kind: "CertificateRequest", ListKind: "CertificateRequestList"},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Type: "object", XPreserveUnknownFields: &preserve}},
			}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		crd, err := crdClient.ApiextensionsV1().CustomResourceDefinitions().Get(t.Context(), "certificaterequests.cert-manager.io", metav1.GetOptions{})
		if !assert.NoError(c, err) {
			return
		}
		established := false
		for _, condition := range crd.Status.Conditions {
			established = established || (condition.Type == apiextensionsv1.Established && condition.Status == apiextensionsv1.ConditionTrue)
		}
		assert.True(c, established)
	}, 30*time.Second, 100*time.Millisecond)
}

// createRenderedIssuancePolicy renders the chart's issuance policy with set and creates it, waiting until the API
// server enforces it.
func createRenderedIssuancePolicy(t *testing.T, helmBinary string, config *rest.Config, client kubernetes.Interface, set ...string) {
	t.Helper()

	args := []string{"template", "radius", filepath.Join("..", "..", "..", "deploy", "Chart"), "--namespace", RadiusSystemNamespace,
		"--api-versions", "admissionregistration.k8s.io/v1/ValidatingAdmissionPolicy", "--show-only", "templates/global/certificate-request-policy.yaml"}
	for _, s := range set {
		args = append(args, "--set", s)
	}
	output, err := exec.CommandContext(t.Context(), helmBinary, args...).CombinedOutput()
	require.NoError(t, err, string(output))

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(output), 4096)
	var policy admissionregistrationv1.ValidatingAdmissionPolicy
	require.NoError(t, decoder.Decode(&policy))
	var binding admissionregistrationv1.ValidatingAdmissionPolicyBinding
	require.NoError(t, decoder.Decode(&binding))
	var extra map[string]any
	require.ErrorIs(t, decoder.Decode(&extra), io.EOF)
	require.Equal(t, "radius-certificate-issuance", policy.Name)

	_, err = client.AdmissionregistrationV1().ValidatingAdmissionPolicies().Create(t.Context(), &policy, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = client.AdmissionregistrationV1().ValidatingAdmissionPolicyBindings().Create(t.Context(), &binding, metav1.CreateOptions{})
	require.NoError(t, err)

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		current, err := client.AdmissionregistrationV1().ValidatingAdmissionPolicies().Get(t.Context(), policy.Name, metav1.GetOptions{})
		if assert.NoError(c, err) && current.Status.TypeChecking != nil {
			assert.Empty(c, current.Status.TypeChecking.ExpressionWarnings)
		}
	}, 30*time.Second, 100*time.Millisecond)
	// Admission picks up a new policy and binding from informers, so wait until a reserved request is denied.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		err := createCertificateRequestAs(t, config, "system:serviceaccount:"+RadiusSystemNamespace+":probe", RadiusSystemNamespace, map[string]any{"name": RadiusBootstrapIssuerName})
		assert.ErrorContains(c, err, "CertificateRequestDenied")
	}, 30*time.Second, 200*time.Millisecond)
}

// createCertificateRequestAs creates a CertificateRequest for issuerRef in namespace as user, a member of
// system:authenticated.
func createCertificateRequestAs(t *testing.T, config *rest.Config, user, namespace string, issuerRef map[string]any) error {
	t.Helper()

	impersonated := rest.CopyConfig(config)
	impersonated.Impersonate = rest.ImpersonationConfig{UserName: user, Groups: []string{"system:authenticated"}}
	client, err := dynamic.NewForConfig(impersonated)
	require.NoError(t, err)
	_, err = client.Resource(certificateRequestsResource).Namespace(namespace).Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "cert-manager.io/v1",
		"kind":       "CertificateRequest",
		"metadata":   map[string]any{"generateName": "test-"},
		"spec": map[string]any{
			"request":   "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURSBSRVFVRVNULS0tLS0K",
			"issuerRef": issuerRef,
		},
	}}, metav1.CreateOptions{})
	return err
}
