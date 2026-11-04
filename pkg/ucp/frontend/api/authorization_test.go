// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/ucp"
	authapi "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization"
	"github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

type requestPolicyVerifier struct{}

func (requestPolicyVerifier) VerifyPolicyInitialization(context.Context, authorization.Policy) error {
	return nil
}

func frontendRequestPolicy(t *testing.T, db database.Client, mode authapi.UserAuthorizationMode) (*authorization.PolicyStore, *authorization.PolicyRuntime) {
	t.Helper()
	catalog, err := authorization.NewCatalog(nil)
	require.NoError(t, err)
	store, err := authorization.NewPolicyStore(db, catalog, nil, nil)
	require.NoError(t, err)
	initializer, err := authorization.NewPolicyInitializer(store, requestPolicyVerifier{})
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), authorization.Policy{
		Settings: authapi.AuthorizationSettingsResource{
			ID: to.Ptr("/providers/Radius.Core/authorizationSettings/default"),
			Properties: &authapi.AuthorizationSettingsProperties{
				Mode: &mode, Generation: to.Ptr(int64(0)),
			},
		},
		Assignments: []authapi.RoleAssignmentResource{{
			ID: to.Ptr("/providers/Radius.Core/roleAssignments/admin"),
			Properties: &authapi.RoleAssignmentProperties{
				RoleDefinitionID: to.Ptr("/providers/Radius.Core/roleDefinitions/radius-administrator"),
				Principal: &authapi.Principal{
					Type: to.Ptr(authapi.PrincipalTypeUser), Issuer: to.Ptr("kubernetes:test-cluster"), Subject: to.Ptr("alice"),
				},
				Scope: to.Ptr("/"), CreatedBy: to.Ptr("trusted-test-startup"),
				ManagedBy: &authapi.AssignmentManagement{Kind: to.Ptr(authapi.AssignmentManagementKindImperative)},
			},
		}},
	}))
	runtime, err := authorization.NewPolicyRuntime(store, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
	require.Eventually(t, func() bool { return runtime.Ready() == nil }, time.Second, time.Millisecond)
	return store, runtime
}

func frontendRequestOptions(t *testing.T, pathBase string) *ucp.Options {
	t.Helper()
	options, err := ucp.NewOptions(t.Context(), &ucp.Config{
		Database: databaseprovider.Options{Provider: databaseprovider.TypeInMemory},
		Secrets:  secretprovider.SecretProviderOptions{Provider: secretprovider.TypeInMemorySecret},
		Queue:    queueprovider.QueueProviderOptions{Provider: queueprovider.TypeInmemory, Name: "ucp"},
		Environment: hostoptions.EnvironmentOptions{
			Name: "test", RoleLocation: v1.LocationGlobal,
		},
		Server: hostoptions.ServerOptions{PathBase: pathBase},
		UCP: config.UCPOptions{
			Kind: config.UCPConnectionKindDirect, Direct: &config.UCPDirectConnectionOptions{Endpoint: "http://localhost:65000"},
		},
	})
	require.NoError(t, err)
	return options
}

func TestServiceDryRunRequests(t *testing.T) {
	t.Parallel()
	const pathBase = "/apis/api.ucp.dev/v1alpha3"
	const group = "/planes/radius/local/resourcegroups/team-a"
	options := frontendRequestOptions(t, pathBase)
	db, err := options.DatabaseProvider.GetClient(t.Context())
	require.NoError(t, err)
	store, runtime := frontendRequestPolicy(t, db, authapi.UserAuthorizationModeDryRun)
	options.UserAuthorization = runtime
	options.Config.UserAuthorization.Enabled = true
	caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
	options.Config.Server.TLSCertificateDirectory = t.TempDir()
	options.Config.CallerAuthentication = &authorization.RequestHeaderOptions{
		CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
	}
	core, logs := observer.New(zap.InfoLevel)
	ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
	server, err := NewService(options).Initialize(ctx)
	require.NoError(t, err)
	listener := httptest.NewUnstartedServer(server.Handler)
	listener.Config.BaseContext = func(net.Listener) context.Context { return ctx }
	listener.TLS = server.TLSConfig
	listener.StartTLS()
	defer listener.Close()
	transport := listener.Client().Transport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig.Certificates = []tls.Certificate{certificate}
	client := &http.Client{Transport: transport}
	call := func(method, path, subject, body string) (int, http.Header, string) {
		t.Helper()
		request, err := http.NewRequest(method, listener.URL+pathBase+path, strings.NewReader(body))
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")
		if subject != "" {
			request.Header.Set("X-Remote-User", subject)
		}
		response, err := client.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, response.Header, string(data)
	}
	// Exercise the real UCP write/read controllers, not a proxy-only stand-in.
	code, _, body := call(http.MethodPut, "/planes/radius/local?api-version=2023-10-01-preview", "alice", `{"location":"global","properties":{"resourceProviders":{}}}`)
	require.Equal(t, http.StatusOK, code, body)
	for _, subject := range []string{"alice", "bob", ""} {
		logs.TakeAll()
		code, _, body = call(http.MethodPut, group+"?api-version=2023-10-01-preview", subject, `{"location":"global"}`)
		require.Equal(t, http.StatusOK, code, body)
		var resource map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &resource))
		require.Equal(t, group, resource["id"])
		events := logs.FilterMessage("User authorization request").All()
		require.Len(t, events, 1)
		expected := "allow"
		if subject == "bob" {
			expected = "wouldDeny"
		} else if subject == "" {
			expected = "indeterminate"
		}
		require.Equal(t, expected, events[0].ContextMap()["result"])
	}
	for _, path := range []string{group, "/planes/radius/local/resourcegroups/missing"} {
		allowed, _, allowedBody := call(http.MethodGet, path+"?api-version=2023-10-01-preview", "alice", "")
		denied, _, deniedBody := call(http.MethodGet, path+"?api-version=2023-10-01-preview", "bob", "")
		require.Equal(t, allowed, denied)
		require.Equal(t, allowedBody, deniedBody)
	}

	var downstreamCalls atomic.Int32
	deploymentBody := `{"properties":{"template":{"resources":[]},"parameters":{"secret":{"value":"do-not-log"}}}}`
	protectedDeploymentBody := `{"properties":{"template":{"resources":[{"type":"Radius.Core/roleAssignments","name":"attacker","properties":{}}]}}}`
	policyAttempts := make(chan int, 1)
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downstreamCalls.Add(1)
		require.Empty(t, r.Header.Get("X-Remote-User"))
		if strings.HasSuffix(r.URL.Path, "/write-policy") {
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, protectedDeploymentBody, string(data))
			// Simulate a deployment step returning through UCP. No caller
			// identity is delegated by this observation-only layer.
			code, _, _ := call(http.MethodPut, group+"/providers/Radius.Core/roleAssignments/attacker?api-version=2025-08-01-preview", "", `{"properties":{}}`)
			policyAttempts <- code
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodPut || r.Method == http.MethodPost {
			data, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.Equal(t, deploymentBody, string(data))
			require.Equal(t, "api-version=2025-01-01&probe=unchanged", r.URL.RawQuery)
			w.Header().Set("X-Downstream", "preserved")
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusAccepted)
			} else {
				w.WriteHeader(http.StatusOK)
			}
			_, err = w.Write([]byte("original deployment response"))
			require.NoError(t, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"id": r.URL.Path, "type": "Microsoft.Resources/deployments", "properties": map[string]any{"provisioningState": "Succeeded"},
		}))
	}))
	defer downstream.Close()
	// Register real proxy routing data for an ordinary deployment.
	deployment := resources.MustParse(group + "/providers/Microsoft.Resources/deployments/test")
	typeID, err := datamodel.ResourceTypeIDFromResourceID(deployment)
	require.NoError(t, err)
	locationID, err := datamodel.ResourceProviderLocationIDFromResourceID(deployment, v1.LocationGlobal)
	require.NoError(t, err)
	for id, value := range map[string]any{
		typeID.String(): &datamodel.ResourceType{ID: typeID.String(), Name: "deployments"},
		locationID.String(): &datamodel.Location{ID: locationID.String(), Properties: datamodel.LocationProperties{
			Address: to.Ptr(downstream.URL),
			ResourceTypes: map[string]datamodel.LocationResourceTypeConfiguration{
				"deployments": {APIVersions: map[string]datamodel.LocationAPIVersionConfiguration{"2025-01-01": {}}},
			},
		}},
	} {
		require.NoError(t, db.Save(t.Context(), &database.Object{Metadata: database.Metadata{ID: id}, Data: value}))
	}
	logs.TakeAll()
	code, headers, body := call(http.MethodPost, deployment.String()+"/validate?api-version=2025-01-01&probe=unchanged", "bob", deploymentBody)
	require.Equal(t, http.StatusAccepted, code, body)
	require.Equal(t, "original deployment response", body)
	require.Equal(t, "preserved", headers.Get("X-Downstream"))
	require.Equal(t, int32(1), downstreamCalls.Load())
	code, headers, body = call(http.MethodPut, deployment.String()+"?api-version=2025-01-01&probe=unchanged", "bob", deploymentBody)
	require.Equal(t, http.StatusOK, code, body)
	require.Equal(t, "original deployment response", body)
	require.Equal(t, "preserved", headers.Get("X-Downstream"))
	require.Equal(t, int32(3), downstreamCalls.Load()) // PUT plus the existing tracking GET.
	events := logs.FilterMessage("User authorization request").FilterField(zap.String("permission", "Microsoft.Resources/deployments/write")).All()
	require.Len(t, events, 1)
	require.Equal(t, "wouldDeny", events[0].ContextMap()["result"])
	for _, event := range logs.FilterMessage("Forwarding request to plane").All() {
		require.Contains(t, event.ContextMap(), "resourceId", "downstream logging retains resource context")
	}
	encodedLogs, err := json.Marshal(logs.All())
	require.NoError(t, err)
	require.NotContains(t, string(encodedLogs), "do-not-log")
	require.NotContains(t, string(encodedLogs), "probe=unchanged")
	code, _, body = call(http.MethodPost, deployment.String()+"/write-policy?api-version=2025-01-01", "bob", protectedDeploymentBody)
	require.Equal(t, http.StatusServiceUnavailable, code, body)
	require.Equal(t, http.StatusUnauthorized, <-policyAttempts)
	require.Equal(t, int32(4), downstreamCalls.Load())
	logs.TakeAll()
	code, _, body = call(http.MethodGet, group+"/providers/Example.Test/widgets/a?api-version=2025-01-01", "bob", "")
	require.Equal(t, http.StatusBadRequest, code, body)
	require.Equal(t, "indeterminate", logs.FilterMessage("User authorization request").All()[0].ContextMap()["result"])

	for _, target := range []string{
		"/providers/Radius.Core/authorizationSettings/default",
		group + "/providers/Radius.Core/roleAssignments/attacker",
		group + "/providers/Radius.Core/roleDefinitions/custom",
		group + "/providers/System.Authorization/accessVersions/default",
		// An encoded namespace must not evade the guard on decoded URL.Path.
		group + "/providers/Radius%2eCore/roleAssignments/attacker",
	} {
		for _, subject := range []string{"alice", "bob", ""} {
			code, _, body = call(http.MethodPut, target+"?api-version=2025-08-01-preview", subject, `{"properties":{}}`)
			expected := http.StatusServiceUnavailable
			if subject == "" {
				expected = http.StatusUnauthorized
			}
			require.Equal(t, expected, code, body)
		}
	}
	require.Equal(t, int32(4), downstreamCalls.Load())
	after, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, int64(0), after.Generation())
	policy, err := after.Policy()
	require.NoError(t, err)
	require.Equal(t, authapi.UserAuthorizationModeDryRun, *policy.Settings.Properties.Mode)
	require.Len(t, policy.Assignments, 1)
	require.Empty(t, policy.CustomRoles)
}

func TestServiceProtectedMutationsEveryMode(t *testing.T) {
	t.Parallel()
	caFile, certificate := frontendProxyCertificate(t, "front-proxy-client")
	for _, mode := range []authapi.UserAuthorizationMode{
		authapi.UserAuthorizationModeDisabled, authapi.UserAuthorizationModeDryRun, authapi.UserAuthorizationModeEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			const pathBase = "/apis/api.ucp.dev/v1alpha3"
			options := frontendRequestOptions(t, pathBase)
			options.Config.Server.TLSCertificateDirectory = t.TempDir()
			options.Config.CallerAuthentication = &authorization.RequestHeaderOptions{
				CAFile: caFile, AllowedNames: []string{"front-proxy-client"}, KubernetesIssuer: "kubernetes:test-cluster",
			}
			db, err := options.DatabaseProvider.GetClient(t.Context())
			require.NoError(t, err)
			store, runtime := frontendRequestPolicy(t, db, mode)
			before, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			beforePolicy, err := before.Policy()
			require.NoError(t, err)
			for _, active := range []*authorization.PolicyRuntime{nil, runtime} {
				options.UserAuthorization = active
				core, logs := observer.New(zap.InfoLevel)
				ctx := logr.NewContext(t.Context(), zapr.NewLogger(zap.New(core)))
				server, err := NewService(options).Initialize(ctx)
				require.NoError(t, err)
				listener := httptest.NewUnstartedServer(server.Handler)
				listener.Config.BaseContext = func(net.Listener) context.Context { return ctx }
				listener.TLS = server.TLSConfig
				listener.StartTLS()
				defer listener.Close()
				transport := listener.Client().Transport.(*http.Transport).Clone()
				defer transport.CloseIdleConnections()
				transport.TLSClientConfig.Certificates = []tls.Certificate{certificate}
				client := &http.Client{Transport: transport}
				assertPrivateEvents := func() {
					t.Helper()
					for _, event := range logs.FilterMessage("User authorization request").All() {
						fields := event.ContextMap()
						require.NotContains(t, fields, "resourceId")
						encoded, err := json.Marshal(fields)
						require.NoError(t, err)
						for _, private := range []string{"private-assignment-id", "private-definition-id"} {
							require.NotContains(t, strings.ToLower(string(encoded)), private)
						}
					}
				}
				send := func(method, target, subject, overrideHeader, override string) (int, string) {
					t.Helper()
					query := "?api-version=2025-08-01-preview"
					if overrideHeader == "_method" {
						query += "&_method=" + override
					}
					request, err := http.NewRequest(method, listener.URL+pathBase+target+query, strings.NewReader(`{"properties":{}}`))
					require.NoError(t, err)
					if subject != "" {
						request.Header.Set("X-Remote-User", subject)
					}
					if overrideHeader != "" && overrideHeader != "_method" {
						request.Header.Set(overrideHeader, override)
					}
					response, err := client.Do(request)
					require.NoError(t, err)
					defer response.Body.Close()
					body, err := io.ReadAll(response.Body)
					require.NoError(t, err)
					return response.StatusCode, string(body)
				}
				for _, subject := range []string{"alice", "bob", ""} {
					for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete} {
						for _, target := range []string{
							"/providers/Radius.Core/authorizationSettings/default",
							"/planes/radius/local/providers/Radius.Core/roleDefinitions/private-definition-id",
							"/planes/radius/local/resourcegroups/team-a/providers/Radius.Core/roleAssignments/private-assignment-id",
							"/planes/azure/local/providers/Radius.Core/roleAssignments/private-assignment-id",
							"/planes/radius/local/providers/System.Authorization/policyObjects/test",
							"/planes/radius/local/providers%2FRadius.Core%2FroleAssignments%2Fprivate-assignment-id",
							"/planes/radius/local/providers/%52adius%2eCore/role%41ssignments%2fprivate-assignment-id",
							"/planes/radius/local/providers//Radius.Core//roleAssignments/private-assignment-id",
							"/planes/radius/local/providers/Radius.Core/roleDefinitions/private-definition-id/",
							"/Planes/radius/local/providers/RADIUS.CORE/ROLEASSIGNMENTS/private-assignment-id",
						} {
							for _, overrideHeader := range []string{"", "X-HTTP-Method-Override", "X-HTTP-Method", "X-Method-Override", "_method"} {
								logs.TakeAll()
								status, body := send(method, target, subject, overrideHeader, http.MethodGet)
								expected := http.StatusServiceUnavailable
								if subject == "" {
									expected = http.StatusUnauthorized
								}
								require.Equal(t, expected, status, "%s %s: %s", method, target, body)
								require.Len(t, logs.FilterMessage("User authorization request").All(), 1)
								require.Zero(t, logs.FilterMessage("Forwarding request to plane").Len())
								assertPrivateEvents()
							}
						}
					}
				}
				// Overrides are ignored in both directions. A read cannot become a
				// policy mutation, and the same ordinary handler response is kept.
				for _, target := range []string{
					"/planes/radius/local/providers/Radius.Core/roleAssignments/private-assignment-id",
					"/planes/radius/local/providers/Radius.Core/roleDefinitions/private-definition-id",
				} {
					for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
						logs.TakeAll()
						status, body := send(method, target, "alice", "", "")
						assertPrivateEvents()
						for _, overrideHeader := range []string{"X-HTTP-Method-Override", "X-HTTP-Method", "X-Method-Override", "_method"} {
							logs.TakeAll()
							overriddenStatus, overriddenBody := send(method, target, "alice", overrideHeader, http.MethodPut)
							require.Equal(t, status, overriddenStatus)
							require.Equal(t, body, overriddenBody)
							assertPrivateEvents()
						}
					}
				}
			}
			after, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			afterPolicy, err := after.Policy()
			require.NoError(t, err)
			require.Equal(t, beforePolicy, afterPolicy)
		})
	}
}
