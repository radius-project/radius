// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestServiceKubernetesRequestHeader(t *testing.T) {
	t.Parallel()
	caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
	ca, err := os.ReadFile(caFile)
	require.NoError(t, err)
	for _, mode := range []authz.Mode{authz.ModeOff, authz.ModeDryRun, authz.ModeEnforce} {
		t.Run(string(mode), func(t *testing.T) {
			client := fake.NewClientset(&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: "extension-apiserver-authentication", Namespace: metav1.NamespaceSystem},
				Data: map[string]string{
					"requestheader-client-ca-file":       string(ca),
					"requestheader-allowed-names":        `["front-proxy-client"]`,
					"requestheader-username-headers":     `["X-Remote-User"]`,
					"requestheader-group-headers":        `["X-Remote-Group"]`,
					"requestheader-extra-headers-prefix": `["X-Remote-Extra-"]`,
				},
			})
			config := &ucp.Config{
				Authorization:                  authz.Options{Mode: mode},
				KubernetesCallerAuthentication: &authorization.KubernetesRequestHeaderOptions{KubernetesIssuer: "kubernetes:cluster-a"},
				Server: hostoptions.ServerOptions{
					TLSCertificateDirectory: t.TempDir(), PathBase: "/apis/api.ucp.dev/v1alpha3",
				},
			}
			service := NewService(identityTestOptions(t, config, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				identity, ok := authorization.IdentityFromContext(r.Context())
				require.True(t, ok)
				require.Equal(t, "alice", *identity.Principal.Subject)
				require.Equal(t, "kubernetes:cluster-a", *identity.Principal.Issuer)
				require.Empty(t, r.Header.Get("X-Remote-User"))
				w.WriteHeader(http.StatusNoContent)
			})))
			service.loadRequestHeaderConfigMap = func(ctx context.Context) (*corev1.ConfigMap, error) {
				return client.CoreV1().ConfigMaps(metav1.NamespaceSystem).Get(ctx, "extension-apiserver-authentication", metav1.GetOptions{})
			}
			server, err := service.Initialize(t.Context())
			require.NoError(t, err)
			listener := httptest.NewUnstartedServer(server.Handler)
			listener.TLS = server.TLSConfig
			listener.StartTLS()
			defer listener.Close()
			transport := listener.Client().Transport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			transport.TLSClientConfig.Certificates = []tls.Certificate{certificate}
			request, err := http.NewRequest(http.MethodGet, listener.URL+config.Server.PathBase+"/planes/identity/local", nil)
			require.NoError(t, err)
			request.Header.Set("X-Remote-User", "alice")
			response, err := (&http.Client{Transport: transport}).Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, http.StatusNoContent, response.StatusCode)
			for _, user := range []string{"system:aggregator", "system:serviceaccount:radius-system:radius-api-readiness"} {
				t.Run(user, func(t *testing.T) {
					request, err := http.NewRequest(http.MethodGet, listener.URL+config.Server.PathBase, nil)
					require.NoError(t, err)
					request.Header.Set("X-Remote-User", user)
					response, err := (&http.Client{Transport: transport}).Do(request)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, http.StatusOK, response.StatusCode, "aggregation discovery and the install readiness hook must remain available")
				})
			}
			require.Len(t, client.Actions(), 1, "trust is a single startup snapshot")
		})
	}
}

func TestServiceKubernetesRequestHeaderErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name     string
		config   ucp.Config
		loadErr  error
		expected string
	}{
		{name: "both sources", config: ucp.Config{
			CallerAuthentication:           &authorization.RequestHeaderOptions{},
			KubernetesCallerAuthentication: &authorization.KubernetesRequestHeaderOptions{},
		}, expected: "mutually exclusive"},
		{name: "HTTP is not permitted", config: ucp.Config{
			KubernetesCallerAuthentication: &authorization.KubernetesRequestHeaderOptions{},
		}, expected: "requires a TLS server certificate directory"},
		{name: "API forbidden is not hidden", config: ucp.Config{
			KubernetesCallerAuthentication: &authorization.KubernetesRequestHeaderOptions{},
			Server:                         hostoptions.ServerOptions{TLSCertificateDirectory: t.TempDir()},
		}, loadErr: errors.New("configmaps is forbidden"), expected: "load Kubernetes request-header trust: configmaps is forbidden"},
		{name: "invalid published trust fails startup", config: ucp.Config{
			KubernetesCallerAuthentication: &authorization.KubernetesRequestHeaderOptions{},
			Server:                         hostoptions.ServerOptions{TLSCertificateDirectory: t.TempDir()},
		}, expected: "configure Kubernetes request-header trust: Kubernetes request-header ConfigMap is missing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			service := NewService(identityTestOptions(t, &tt.config, nil))
			service.loadRequestHeaderConfigMap = func(context.Context) (*corev1.ConfigMap, error) { return nil, tt.loadErr }
			server, err := service.Initialize(t.Context())
			require.Nil(t, server)
			require.ErrorContains(t, err, tt.expected)
			if tt.loadErr != nil {
				require.ErrorIs(t, err, tt.loadErr)
			}
		})
	}
}

func TestLoadRequestHeaderConfigMapRequiresInClusterConfig(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	_, err := loadRequestHeaderConfigMap(t.Context())
	require.ErrorContains(t, err, "create in-cluster request-header client configuration")
}

func TestRequestHeaderConfigMapAPI(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusForbidden, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "/api/v1/namespaces/kube-system/configmaps/extension-apiserver-authentication", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status != http.StatusOK {
					require.NoError(t, json.NewEncoder(w).Encode(metav1.Status{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Status"},
						Status:   "Failure", Message: "published trust is unavailable", Code: int32(status),
					}))
					return
				}
				require.NoError(t, json.NewEncoder(w).Encode(corev1.ConfigMap{
					TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
					ObjectMeta: metav1.ObjectMeta{Name: "extension-apiserver-authentication", Namespace: metav1.NamespaceSystem},
					Data:       map[string]string{"requestheader-client-ca-file": "public proxy trust"},
				}))
			}))
			defer backend.Close()
			config := &rest.Config{
				Host: backend.URL,
				TLSClientConfig: rest.TLSClientConfig{
					CAData: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: backend.Certificate().Raw}),
				},
			}
			configMap, err := requestHeaderConfigMap(t.Context(), config)
			require.Zero(t, config.Timeout, "loading trust must not mutate the caller's client configuration")
			if status == http.StatusOK {
				require.NoError(t, err)
				require.Equal(t, "public proxy trust", configMap.Data["requestheader-client-ca-file"])
			} else {
				require.Nil(t, configMap)
				require.ErrorContains(t, err, "get kube-system/extension-apiserver-authentication")
				require.ErrorContains(t, err, "published trust is unavailable")
			}
		})
	}
	_, err := requestHeaderConfigMap(t.Context(), &rest.Config{Host: "https://invalid\n"})
	require.ErrorContains(t, err, "create request-header Kubernetes client")
}
