//go:build !windows

// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/cli/cmd/auth"
	"github.com/radius-project/radius/pkg/cli/framework"
	"github.com/radius-project/radius/pkg/cli/output"
	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func aggregationServingCertificate(t *testing.T, address string) (tls.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	certificate := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ucp.default.svc"},
		DNSNames: []string{"ucp.default.svc"}, IPAddresses: []net.IP{net.ParseIP(address)},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, certificate, certificate, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestInspectionKubernetesAggregation(t *testing.T) {
	if testing.Short() {
		t.Skip("starts isolated kube-apiserver and etcd; run without -short with KUBEBUILDER_ASSETS")
	}
	// Never honor USE_EXISTING_CLUSTER for this test: its APIService, RBAC,
	// authentication configuration, and policy are exclusively test-owned.
	environment := &envtest.Environment{UseExistingCluster: to.Ptr(false)}
	caFile, proxyCertificate := frontendProxyCertificate(t, "front-proxy-client")
	directory := t.TempDir()
	proxyCertFile, proxyKeyFile := filepath.Join(directory, "proxy.crt"), filepath.Join(directory, "proxy.key")
	require.NoError(t, os.WriteFile(proxyCertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: proxyCertificate.Certificate[0]}), 0600))
	key, err := x509.MarshalPKCS8PrivateKey(proxyCertificate.PrivateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(proxyKeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0600))
	environment.ControlPlane.GetAPIServer().Configure().
		Set("requestheader-client-ca-file", caFile).
		Set("requestheader-allowed-names", "front-proxy-client").
		Set("requestheader-username-headers", "X-Remote-User").
		Set("requestheader-group-headers", "X-Remote-Group").
		Set("requestheader-extra-headers-prefix", "X-Remote-Extra-").
		Set("proxy-client-cert-file", proxyCertFile).
		Set("proxy-client-key-file", proxyKeyFile)
	adminConfig, err := environment.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, environment.Stop()) })
	admin, err := kubernetes.NewForConfig(adminConfig)
	require.NoError(t, err)

	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	options := frontendRequestOptions(t, pathBase)
	db, err := options.DatabaseProvider.GetClient(t.Context())
	require.NoError(t, err)
	_, runtime := frontendRequestPolicy(t, db, authapi.UserAuthorizationModeDryRun)
	options.UserAuthorization = runtime
	options.Config.Server.TLSCertificateDirectory = directory
	options.Config.CallerAuthentication = &authorization.RequestHeaderOptions{
		CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
	}
	actor := authorization.PolicyActor{
		Identity: authorization.Identity{Principal: authapi.Principal{
			Type: to.Ptr(authapi.PrincipalTypeUser), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: to.Ptr("alice"),
		}},
		Management: authapi.AssignmentManagement{Kind: to.Ptr(authapi.AssignmentManagementKindImperative)},
	}
	for i, group := range []string{"developers", "system:serviceaccounts:default"} {
		actor.RequestID = fmt.Sprintf("fixture-%d", i)
		require.NoError(t, runtime.Mutate(t.Context(), actor, authorization.PolicyMutation{
			Target: fmt.Sprintf("/providers/Radius.Core/roleAssignments/reader-%d", i),
			Assignment: &authapi.RoleAssignmentProperties{
				Principal:        &authapi.Principal{Type: to.Ptr(authapi.PrincipalTypeGroup), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: &group},
				RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/reader"), Scope: to.Ptr("/"),
			},
		}))
	}
	server, err := NewService(options).Initialize(t.Context())
	require.NoError(t, err)
	const address = "127.0.0.1"
	listener, err := net.Listen("tcp4", net.JoinHostPort(address, "0"))
	require.NoError(t, err)
	servingCert, caBundle := aggregationServingCertificate(t, address)
	ucp := httptest.NewUnstartedServer(server.Handler)
	require.NoError(t, ucp.Listener.Close())
	ucp.Listener = listener
	ucp.TLS = server.TLSConfig.Clone()
	ucp.TLS.Certificates = []tls.Certificate{servingCert}
	ucp.StartTLS()
	t.Cleanup(ucp.Close)
	port := int32(listener.Addr().(*net.TCPAddr).Port)

	_, err = admin.CoreV1().Services("default").Create(t.Context(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "ucp", Namespace: "default"},
		// The real aggregator supports ExternalName services. Loopback DNS
		// keeps all traffic local without kube-proxy or invalid loopback endpoints.
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeExternalName, ExternalName: "localhost",
			Ports: []corev1.ServicePort{{Name: "https", Port: port, TargetPort: intstr.FromInt32(port)}}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	dynamicClient, err := dynamic.NewForConfig(adminConfig)
	require.NoError(t, err)
	apiServices := dynamicClient.Resource(schema.GroupVersionResource{Group: "apiregistration.k8s.io", Version: "v1", Resource: "apiservices"})
	_, err = apiServices.Create(t.Context(), &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]any{"name": "v1alpha3.api.ucp.dev"},
		"spec": map[string]any{
			"group": "api.ucp.dev", "version": "v1alpha3", "groupPriorityMinimum": int64(1000), "versionPriority": int64(15),
			"service":  map[string]any{"namespace": "default", "name": "ucp", "port": int64(port)},
			"caBundle": caBundle,
		},
	}}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = admin.RbacV1().ClusterRoles().Create(t.Context(), &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "radius-inspection"},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"api.ucp.dev"}, Resources: []string{"*"}, Verbs: []string{"get", "create"}},
			{NonResourceURLs: []string{"/apis/api.ucp.dev", "/apis/api.ucp.dev/*"}, Verbs: []string{"get"}},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = admin.RbacV1().ClusterRoleBindings().Create(t.Context(), &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "radius-inspection"},
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: "radius-inspection"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", APIGroup: rbacv1.GroupName, Name: "system:authenticated"}},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = admin.CoreV1().ServiceAccounts("default").Create(t.Context(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "inspection"}}, metav1.CreateOptions{})
	require.NoError(t, err)
	token, err := admin.CoreV1().ServiceAccounts("default").CreateToken(t.Context(), "inspection", &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: to.Ptr(int64(600))},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	serviceConfig := rest.AnonymousClientConfig(adminConfig)
	serviceConfig.BearerToken = token.Status.Token
	alice, err := environment.ControlPlane.AddUser(envtest.User{Name: "alice", Groups: []string{"developers"}}, adminConfig)
	require.NoError(t, err)
	bob, err := environment.ControlPlane.AddUser(envtest.User{Name: "bob", Groups: []string{"developers"}}, adminConfig)
	require.NoError(t, err)
	kubeconfig := filepath.Join(directory, "kubeconfig")
	t.Setenv("KUBECONFIG", kubeconfig)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		response, err := admin.RESTClient().Get().AbsPath(pathBase).DoRaw(t.Context())
		require.NoError(collect, err, string(response))
	}, 30*time.Second, 100*time.Millisecond)

	for _, identity := range []struct {
		name, typ, group string
		config           *rest.Config
		admin            bool
	}{
		{"alice", "user", "developers", alice.Config(), true},
		{"bob", "user", "developers", bob.Config(), false},
		{"system:serviceaccount:default:inspection", "workload", "system:serviceaccounts:default", serviceConfig, false},
	} {
		t.Run(identity.name, func(t *testing.T) {
			require.NoError(t, clientcmd.WriteToFile(clientcmdapi.Config{
				Clusters: map[string]*clientcmdapi.Cluster{"isolated": {Server: strings.TrimSuffix(identity.config.Host, "/"), CertificateAuthorityData: identity.config.CAData}},
				AuthInfos: map[string]*clientcmdapi.AuthInfo{"caller": {
					ClientCertificateData: identity.config.CertData, ClientKeyData: identity.config.KeyData, Token: identity.config.BearerToken,
				}},
				Contexts: map[string]*clientcmdapi.Context{"isolated": {Cluster: "isolated", AuthInfo: "caller"}}, CurrentContext: "isolated",
			}, kubeconfig))
			runCLI := func(args ...string) (map[string]any, error) {
				t.Helper()
				config := viper.New()
				config.Set("workspaces.default", "isolated")
				config.Set("workspaces.items.isolated.connection", map[string]any{"kind": "kubernetes", "context": "isolated"})
				config.Set("workspaces.items.isolated.scope", "/planes/radius/local/resourceGroups/test")
				var buffer bytes.Buffer
				command := auth.NewCommand(&framework.Impl{ConfigHolder: &framework.ConfigHolder{Config: config}, Output: &output.OutputWriter{Writer: &buffer}})
				command.SilenceUsage, command.SilenceErrors = true, true
				command.SetArgs(append(args, "--output", "json"))
				err := command.ExecuteContext(t.Context())
				var result map[string]any
				if err == nil {
					require.NoError(t, json.Unmarshal(buffer.Bytes(), &result), buffer.String())
				} else {
					require.Empty(t, buffer.String())
				}
				return result, err
			}
			current, err := runCLI("whoami")
			require.NoError(t, err)
			principal := current["principal"].(map[string]any)
			require.Equal(t, identity.name, principal["subject"])
			require.Equal(t, identity.typ, principal["type"])
			require.Equal(t, "kubernetes:test-cluster", current["issuer"])
			require.Equal(t, identity.admin, current["isRadiusAdministrator"])
			groups := current["groups"].([]any)
			found := false
			for _, group := range groups {
				found = found || group.(map[string]any)["subject"] == identity.group
			}
			require.True(t, found, groups)
			status, err := runCLI("status")
			require.NoError(t, err)
			require.Equal(t, "DryRun", status["mode"])
			require.NotEmpty(t, status["warnings"])
			check, err := runCLI("access", "check", "--action", "Radius.Core/applications/read", "--action", "Radius.Core/applications/write")
			require.NoError(t, err)
			require.Equal(t, float64(2), check["generation"])
			checks := check["checks"].([]any)
			require.Equal(t, true, checks[0].(map[string]any)["allowed"])
			require.Equal(t, identity.admin, checks[1].(map[string]any)["allowed"])
			batchArgs := []string{"access", "check"}
			for range 100 {
				batchArgs = append(batchArgs, "--action", "Radius.Core/applications/read")
			}
			boundary, err := runCLI(batchArgs...)
			require.NoError(t, err)
			require.Len(t, boundary["checks"], 100)
			_, err = runCLI(append(batchArgs, "--action", "Radius.Core/applications/read")...)
			require.ErrorContains(t, err, "InvalidRequestContent")
			require.ErrorContains(t, err, "checks must contain between 1 and 100 items")
			_, err = runCLI("access", "check", "--action", strings.Repeat("x", 257))
			require.ErrorContains(t, err, "permission must contain at most 256 characters")
			self, err := runCLI("access", "check", "--action", "Radius.Core/applications/read", "--principal", identity.typ+":"+identity.name)
			require.NoError(t, err)
			require.Equal(t, true, self["checks"].([]any)[0].(map[string]any)["allowed"])
			groupCheck, err := runCLI("access", "check", "--action", "Radius.Core/applications/read", "--principal", "group:developers")
			if identity.admin {
				require.NoError(t, err)
				require.Equal(t, true, groupCheck["checks"].([]any)[0].(map[string]any)["allowed"])
			} else {
				require.ErrorContains(t, err, "AuthorizationFailed")
			}
			_, err = runCLI("access", "check", "--action", "Radius.Core/applications/read", "--principal", "user:someone-else")
			require.Error(t, err)
			if identity.admin {
				require.Contains(t, err.Error(), "AuthorizationIndeterminate")
			} else {
				require.Contains(t, err.Error(), "AuthorizationFailed")
			}

			// Forged forwarded headers at the real Kubernetes endpoint must be
			// replaced by the authenticating proxy, never reach UCP as claims.
			client, err := rest.HTTPClientFor(identity.config)
			require.NoError(t, err)
			request, err := http.NewRequestWithContext(t.Context(), "GET", strings.TrimSuffix(identity.config.Host, "/")+pathBase+"/providers/Radius.Core/whoAmI?api-version=2025-08-01-preview", nil)
			require.NoError(t, err)
			request.Header.Set("X-Remote-User", "mallory")
			request.Header.Set("X-Remote-Group", "admins")
			response, err := client.Do(request)
			require.NoError(t, err)
			defer response.Body.Close()
			data, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode, string(data))
			require.NotContains(t, string(data), "mallory")
			require.NotContains(t, string(data), "admins")
			require.Contains(t, string(data), identity.name)
			t.Logf("real aggregation: %s (%s), verified group %s; CLI whoami/status/batch checks and forged-header replacement passed", identity.name, identity.typ, identity.group)
		})
	}
	// No client certificate, or a certificate from a different front-proxy CA,
	// cannot assert identity even when connecting straight to the UCP listener.
	_, untrustedCertificate := frontendProxyCertificate(t, "front-proxy-client")
	for _, certificate := range []*tls.Certificate{nil, &untrustedCertificate} {
		transport := ucp.Client().Transport.(*http.Transport).Clone()
		transport.TLSClientConfig.RootCAs = x509.NewCertPool()
		require.True(t, transport.TLSClientConfig.RootCAs.AppendCertsFromPEM(caBundle))
		transport.TLSClientConfig.InsecureSkipVerify = false
		if certificate != nil {
			transport.TLSClientConfig.Certificates = []tls.Certificate{*certificate}
		}
		client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
		request, err := http.NewRequest("GET", ucp.URL+pathBase+"/providers/Radius.Core/whoAmI?api-version=2025-08-01-preview", nil)
		require.NoError(t, err)
		request.Header.Set("X-Remote-User", "alice")
		response, err := client.Do(request)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, response.StatusCode)
		require.NoError(t, response.Body.Close())
		transport.CloseIdleConnections()
	}
	t.Log("isolated APIService used verified serving CA and dedicated front-proxy CA; direct missing/untrusted proxy identity rejected; shared cluster untouched")
}
