// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func kubernetesRequestHeaderConfigMap(t *testing.T, caFile string) *corev1.ConfigMap {
	t.Helper()
	ca, err := os.ReadFile(caFile)
	require.NoError(t, err)
	return &corev1.ConfigMap{Data: map[string]string{
		"requestheader-client-ca-file":       string(ca),
		"requestheader-allowed-names":        `["front-proxy-client"]`,
		"requestheader-username-headers":     `["X-Remote-User"]`,
		"requestheader-group-headers":        `["X-Remote-Group"]`,
		"requestheader-extra-headers-prefix": `["X-Remote-Extra-"]`,
	}}
}

func TestKubernetesRequestHeaderTrust(t *testing.T) {
	t.Parallel()
	proxy := newProxyPKI(t)
	userCA := newProxyPKI(t)
	for _, tt := range []struct {
		name        string
		allowed     string
		certificate *x509.Certificate
		wantErr     string
	}{
		{name: "verified proxy", allowed: `["front-proxy-client"]`, certificate: proxy.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)},
		{name: "empty names accepts dedicated CA", allowed: `[]`, certificate: proxy.client(t, "other-proxy", x509.ExtKeyUsageClientAuth, false)},
		{name: "wrong proxy name", allowed: `["front-proxy-client"]`, certificate: proxy.client(t, "other-proxy", x509.ExtKeyUsageClientAuth, false), wantErr: "not an allowed"},
		{name: "general client CA is not proxy trust", allowed: `[]`, certificate: userCA.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false), wantErr: "unknown authority"},
		{name: "expired proxy", allowed: `[]`, certificate: proxy.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, true), wantErr: "expired"},
		{name: "server certificate", allowed: `[]`, certificate: proxy.client(t, "front-proxy-client", x509.ExtKeyUsageServerAuth, false), wantErr: "incompatible key usage"},
		{name: "unrestricted EKU", allowed: `[]`, certificate: proxy.client(t, "front-proxy-client", x509.ExtKeyUsageAny, false), wantErr: "explicitly permit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configMap := kubernetesRequestHeaderConfigMap(t, proxy.file)
			generalCA, err := os.ReadFile(userCA.file)
			require.NoError(t, err)
			configMap.Data["client-ca-file"] = string(generalCA)
			configMap.Data["requestheader-allowed-names"] = tt.allowed
			authenticator, err := NewKubernetesRequestHeaderAuthenticator(configMap, KubernetesRequestHeaderOptions{KubernetesIssuer: "kubernetes:cluster-a"})
			require.NoError(t, err)
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.TLS = &tls.ConnectionState{
				PeerCertificates: []*x509.Certificate{tt.certificate},
				// TLS chains from another trust domain must not influence identity.
				VerifiedChains: [][]*x509.Certificate{{tt.certificate, userCA.ca}},
			}
			request.Header.Set("X-Remote-User", "alice")
			request.Header.Add("X-Remote-Group", "developers")
			identity, err := authenticator.Authenticate(request)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, "alice", *identity.Principal.Subject)
			require.Equal(t, "kubernetes:cluster-a", *identity.Principal.Issuer)
			require.Equal(t, "developers", *identity.Groups[0].Subject)
		})
	}
}

func TestKubernetesRequestHeaderInvalidConfiguration(t *testing.T) {
	t.Parallel()
	proxy := newProxyPKI(t)
	for _, tt := range []struct {
		name  string
		field string
		value string
		err   string
	}{
		{name: "missing proxy CA", field: "requestheader-client-ca-file", err: "contains no certificates"},
		{name: "invalid proxy CA", field: "requestheader-client-ca-file", value: "invalid", err: "contains no certificates"},
		{name: "missing names", field: "requestheader-allowed-names", err: "parse Kubernetes requestheader-allowed-names"},
		{name: "null names", field: "requestheader-allowed-names", value: "null", err: "not null"},
		{name: "invalid names", field: "requestheader-allowed-names", value: "[", err: "parse Kubernetes"},
		{name: "blank name", field: "requestheader-allowed-names", value: `[" "]`, err: "empty names"},
		{name: "missing username", field: "requestheader-username-headers", err: "parse Kubernetes"},
		{name: "custom username", field: "requestheader-username-headers", value: `["X-User"]`, err: "must contain only"},
		{name: "multiple usernames", field: "requestheader-username-headers", value: `["X-Remote-User","X-User"]`, err: "must contain only"},
		{name: "empty groups", field: "requestheader-group-headers", value: `[]`, err: "must contain only"},
		{name: "custom extras", field: "requestheader-extra-headers-prefix", value: `["X-Extra-"]`, err: "must contain only"},
		{name: "custom UID", field: "requestheader-uid-headers", value: `["X-Uid"]`, err: "must be empty or contain only"},
		{name: "multiple UIDs", field: "requestheader-uid-headers", value: `["X-Remote-Uid","X-Uid"]`, err: "must be empty or contain only"},
		{name: "null UID config", field: "requestheader-uid-headers", value: `null`, err: "not null"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configMap := kubernetesRequestHeaderConfigMap(t, proxy.file)
			configMap.Data[tt.field] = tt.value
			// Never fall back to the general Kubernetes client CA.
			configMap.Data["client-ca-file"] = configMap.Data["requestheader-client-ca-file"]
			_, err := NewKubernetesRequestHeaderAuthenticator(configMap, KubernetesRequestHeaderOptions{KubernetesIssuer: "kubernetes:cluster-a"})
			require.ErrorContains(t, err, tt.err)
		})
	}
	_, err := NewKubernetesRequestHeaderAuthenticator(nil, KubernetesRequestHeaderOptions{})
	require.ErrorContains(t, err, "ConfigMap is missing")
	_, err = NewKubernetesRequestHeaderAuthenticator(kubernetesRequestHeaderConfigMap(t, proxy.file), KubernetesRequestHeaderOptions{})
	require.ErrorContains(t, err, "requires a kubernetes:<trust-domain> issuer")
}

func TestKubernetesRequestHeaderSnapshot(t *testing.T) {
	t.Parallel()
	proxy := newProxyPKI(t)
	replacement := newProxyPKI(t)
	configMap := kubernetesRequestHeaderConfigMap(t, proxy.file)
	configMap.Data["requestheader-username-headers"] = `["x-remote-user"]`
	configMap.Data["requestheader-uid-headers"] = `["x-remote-uid"]`
	authenticator, err := NewKubernetesRequestHeaderAuthenticator(configMap, KubernetesRequestHeaderOptions{KubernetesIssuer: "kubernetes:original"})
	require.NoError(t, err)
	originalCertificate := proxy.client(t, "front-proxy-client", x509.ExtKeyUsageClientAuth, false)
	replacementCertificate := replacement.client(t, "new-proxy", x509.ExtKeyUsageClientAuth, false)
	newCA, err := os.ReadFile(replacement.file)
	require.NoError(t, err)
	configMap.Data["requestheader-client-ca-file"] = string(newCA)
	configMap.Data["requestheader-allowed-names"] = `["new-proxy"]`
	refreshed, err := NewKubernetesRequestHeaderAuthenticator(configMap, KubernetesRequestHeaderOptions{KubernetesIssuer: "kubernetes:replacement"})
	require.NoError(t, err)
	for _, tt := range []struct {
		authenticator *RequestHeaderAuthenticator
		certificate   *x509.Certificate
		issuer        string
	}{
		{authenticator, originalCertificate, "kubernetes:original"},
		{authenticator, replacementCertificate, ""},
		{refreshed, originalCertificate, ""},
		{refreshed, replacementCertificate, "kubernetes:replacement"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{tt.certificate}}
		request.Header.Set("X-Remote-User", "alice")
		identity, err := tt.authenticator.Authenticate(request)
		if tt.issuer == "" {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, tt.issuer, *identity.Principal.Issuer)
		}
	}
}
