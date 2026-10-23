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

package processors

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources/v3"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/require"
)

func Test_Delete_ARM_LockExistenceDiagnostics(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		code       string
		existence  string
		qualified  bool
		wantError  bool
		wantDelete bool
	}{
		{name: "present", status: 200, existence: "present", wantDelete: true},
		{name: "Azure qualified", status: 200, existence: "present", qualified: true, wantDelete: true},
		{name: "absent", status: 404, code: "ResourceNotFound", existence: "absent"},
		{name: "parent absent", status: 404, code: "ParentResourceNotFound", existence: "absent"},
		{name: "resource group absent", status: 404, code: "ResourceGroupNotFound", existence: "absent"},
		{name: "forbidden does not prove absence", status: 403, code: "AuthorizationFailed", existence: "unknown", wantError: true},
		{name: "unknown route does not prove absence", status: 404, code: "UnknownRoute", existence: "unknown", wantError: true},
		{name: "invalid API version", status: 400, code: "InvalidApiVersionParameter", existence: "unknown", wantError: true},
		{name: "service failure", status: 500, code: "InternalServerError", existence: "unknown", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var calls []string
			var mu sync.Mutex
			mux := http.NewServeMux()
			mux.HandleFunc(ARMExtensionProviderPath, handleJSONResponse(t, armresources.Provider{
				ResourceTypes: []*armresources.ProviderResourceType{{
					ResourceType: new("locks"), APIVersions: []*string{new(ARMAPIVersion)},
				}},
			}, http.StatusOK))
			mux.HandleFunc(ARMExtensionResourceID, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, r.Method)
				if r.Method == http.MethodGet {
					if tt.code != "" {
						handleJSONResponse(t, map[string]any{
							"error": map[string]string{"code": tt.code, "message": "sensitive-response-body"},
						}, tt.status)(w, r)
					} else {
						handleJSONResponse(t, map[string]any{
							"id": ARMExtensionResourceID, "properties": map[string]string{"notes": "sensitive-response-body"},
						}, tt.status)(w, r)
					}
					return
				}
				require.Equal(t, http.MethodDelete, r.Method)
				w.WriteHeader(http.StatusNoContent)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			c := NewResourceClient(newArmOptions(server.URL), nil, nil)
			c.armClientOptions = newClientOptions(server.Client(), server.URL)
			c.armClientOptions.Retry.MaxRetries = -1
			var logs []string
			ctx := logr.NewContext(t.Context(), funcr.NewJSON(func(line string) {
				logs = append(logs, line)
			}, funcr.Options{}))
			id := ARMExtensionResourceID
			if tt.qualified {
				id = "/planes/azure/azurecloud" + id
			}

			err := c.Delete(ctx, id)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Len(t, logs, 1)
			require.NotContains(t, logs[0], "sensitive-response-body")
			var diagnostic map[string]any
			require.NoError(t, json.Unmarshal([]byte(logs[0]), &diagnostic))
			require.Equal(t, "Azure lock existence check before deletion", diagnostic["msg"])
			require.Equal(t, ARMExtensionResourceID, diagnostic["resourceID"])
			require.Equal(t, ARMAPIVersion, diagnostic["apiVersion"])
			require.Equal(t, tt.existence, diagnostic["existence"])
			require.Equal(t, float64(tt.status), diagnostic["statusCode"])
			require.Equal(t, tt.code, diagnostic["errorCode"])
			expectedCalls := []string{http.MethodGet}
			if tt.wantDelete {
				expectedCalls = append(expectedCalls, http.MethodDelete)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, expectedCalls, calls)
		})
	}
}

func Test_lookupARMAPIVersions(t *testing.T) {
	tests := []struct {
		name     string
		versions []*string
		def      *string
		expected []string
	}{
		{
			name:     "advertised default first",
			versions: []*string{new("2026-10-01"), new("2025-08-01"), new("2024-08-01")},
			def:      new("2025-08-01"), expected: []string{"2025-08-01", "2026-10-01", "2024-08-01"},
		},
		{
			name:     "absent default preserves order",
			versions: []*string{new("2026-10-01"), new("2025-08-01")},
			expected: []string{"2026-10-01", "2025-08-01"},
		},
		{
			name:     "unadvertised default excluded",
			versions: []*string{new("2025-08-01")}, def: new("2026-10-01"),
			expected: []string{"2025-08-01"},
		},
		{
			name:     "empty default excluded",
			versions: []*string{new("2025-08-01")}, def: new(""),
			expected: []string{"2025-08-01"},
		},
		{
			name:     "nil empty and duplicate versions excluded",
			versions: []*string{nil, new(""), new(" "), new("2025-08-01"), new("2025-08-01")},
			expected: []string{"2025-08-01"},
		},
		{name: "default alone does not establish support", def: new("2026-10-01")},
		{name: "no versions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
				ResourceTypes: []*armresources.ProviderResourceType{
					nil, {},
					{ResourceType: new("virtualMachines"), DefaultAPIVersion: tt.def, APIVersions: tt.versions},
				},
			}, http.StatusOK))
			server := httptest.NewServer(mux)
			defer server.Close()
			c := NewResourceClient(newArmOptions(server.URL), nil, nil)
			c.armClientOptions = newClientOptions(server.Client(), server.URL)
			id, err := resources.ParseResource(ARMResourceID)
			require.NoError(t, err)

			actual, err := c.lookupARMAPIVersions(t.Context(), id)
			if tt.expected == nil {
				require.ErrorContains(t, err, "no supported API versions")
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, actual)
			}
		})
	}
}

func Test_Delete_ARM_VersionFallback(t *testing.T) {
	const (
		current          = "2026-10-01"
		older            = "2025-08-01"
		childID          = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.DBforPostgreSQL/flexibleServers/server/firewallRules/allow-all"
		roleAssignmentID = "/subscriptions/0000/resourceGroups/test-rg/providers/Microsoft.Authorization/roleAssignments/test-assignment"
	)
	type failure struct {
		status int
		code   string
	}
	tests := []struct {
		name          string
		id            string
		readFailure   failure
		deleteFailure failure
		pollFailure   bool
		allRejected   bool
		want          []string
		wantError     string
	}{
		{name: "valid default", want: []string{"GET " + current, "DELETE " + current}},
		{
			name: "default unavailable in target region", id: childID,
			readFailure: failure{400, "NoRegisteredProviderFound"},
			want:        []string{"GET " + current, "GET " + older, "DELETE " + older},
		},
		{
			name: "invalid default version", readFailure: failure{400, "InvalidApiVersionParameter"},
			want: []string{"GET " + current, "GET " + older, "DELETE " + older},
		},
		{
			name: "delete rejects version accepted by get", id: AzureUCPResourceID,
			deleteFailure: failure{400, "NoRegisteredProviderFound"},
			want:          []string{"GET " + current, "DELETE " + current, "GET " + older, "DELETE " + older},
		},
		{
			name: "no supported read candidate", allRejected: true,
			readFailure: failure{400, "NoRegisteredProviderFound"},
			want:        []string{"GET " + current, "GET " + older}, wantError: "NoRegisteredProviderFound",
		},
		{
			name: "no supported delete candidate", allRejected: true,
			deleteFailure: failure{400, "InvalidApiVersionParameter"},
			want:          []string{"GET " + current, "DELETE " + current, "GET " + older, "DELETE " + older},
			wantError:     "InvalidApiVersionParameter",
		},
		{
			name: "read permission failure", readFailure: failure{403, "AuthorizationFailed"},
			want: []string{"GET " + current}, wantError: "AuthorizationFailed",
		},
		{
			name: "read server failure", readFailure: failure{500, "InternalServerError"},
			want: []string{"GET " + current}, wantError: "InternalServerError",
		},
		{
			name: "version code without routing rejection status", deleteFailure: failure{500, "NoRegisteredProviderFound"},
			want: []string{"GET " + current, "DELETE " + current}, wantError: "NoRegisteredProviderFound",
		},
		{
			name: "unrelated bad request", readFailure: failure{400, "BadRequest"},
			want: []string{"GET " + current}, wantError: "BadRequest",
		},
		{
			name: "unknown not found route", readFailure: failure{404, "UnknownRoute"},
			want: []string{"GET " + current}, wantError: "UnknownRoute",
		},
		{
			name: "resource already absent", readFailure: failure{404, "ResourceNotFound"},
			want: []string{"GET " + current},
		},
		{
			name: "role assignment already absent", id: roleAssignmentID,
			readFailure: failure{404, "RoleAssignmentNotFound"},
			want:        []string{"GET " + current},
		},
		{
			name: "role assignment bad request remains an error", id: roleAssignmentID,
			readFailure: failure{400, "RoleAssignmentNotFound"},
			want:        []string{"GET " + current}, wantError: "RoleAssignmentNotFound",
		},
		{
			name: "role assignment forbidden remains an error", id: roleAssignmentID,
			readFailure: failure{403, "RoleAssignmentNotFound"},
			want:        []string{"GET " + current}, wantError: "RoleAssignmentNotFound",
		},
		{
			name: "parent already absent", id: childID, readFailure: failure{404, "ParentResourceNotFound"},
			want: []string{"GET " + current},
		},
		{
			name: "resource group already absent", readFailure: failure{404, "ResourceGroupNotFound"},
			want: []string{"GET " + current},
		},
		{
			name: "resource removed after read", deleteFailure: failure{404, "NotFound"},
			want: []string{"GET " + current, "DELETE " + current},
		},
		{
			name: "delete conflict", deleteFailure: failure{409, "Conflict"},
			want: []string{"GET " + current, "DELETE " + current}, wantError: "Conflict",
		},
		{
			name: "uncertain delete outcome", deleteFailure: failure{500, "InternalServerError"},
			want: []string{"GET " + current, "DELETE " + current}, wantError: "InternalServerError",
		},
		{
			name: "do not retry accepted operation", pollFailure: true,
			want: []string{"GET " + current, "DELETE " + current, "POLL"}, wantError: "NoRegisteredProviderFound",
		},
		{
			name: "global resource without region metadata", id: ARMExtensionResourceID,
			want: []string{"GET " + current, "DELETE " + current},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id := tt.id
			if id == "" {
				id = ARMResourceID
			}
			parsed, err := resources.ParseResource(id)
			require.NoError(t, err)
			resourcePath := id
			if parsed.IsUCPQualified() {
				resourcePath = resources.MakeRelativeID(parsed.ScopeSegments()[1:], parsed.TypeSegments(), parsed.ExtensionSegments())
			}
			shortType := parsed.Type()[len(parsed.ProviderNamespace())+1:]
			providerPath := "/subscriptions/0000/providers/" + parsed.ProviderNamespace()
			var calls []string
			var mu sync.Mutex
			mux := http.NewServeMux()
			mux.HandleFunc(providerPath, handleJSONResponse(t, armresources.Provider{
				ResourceTypes: []*armresources.ProviderResourceType{
					{ResourceType: new("flexibleServers"), APIVersions: []*string{new("parent-only-version")}},
					{
						ResourceType: new(shortType), DefaultAPIVersion: new(current),
						APIVersions: []*string{new(older), new(current)},
						// Independent lists cannot establish support for current in Central US.
						Locations: []*string{new("Central US")},
					},
				},
			}, http.StatusOK))
			mux.HandleFunc(resourcePath, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				version := r.URL.Query().Get("api-version")
				calls = append(calls, r.Method+" "+version)
				var rejection failure
				if version == current || tt.allRejected {
					switch r.Method {
					case http.MethodGet:
						rejection = tt.readFailure
					case http.MethodDelete:
						rejection = tt.deleteFailure
					}
				}
				if rejection.code != "" {
					handleJSONResponse(t, map[string]any{
						"error": map[string]string{"code": rejection.code, "message": rejection.code},
					}, rejection.status)(w, r)
					return
				}
				switch r.Method {
				case http.MethodGet:
					resource := armresources.GenericResource{ID: new(resourcePath)}
					if id != ARMExtensionResourceID && id != childID {
						resource.Location = new("centralus")
					}
					handleJSONResponse(t, resource, http.StatusOK)(w, r)
				case http.MethodDelete:
					if tt.pollFailure {
						w.Header().Set("Azure-AsyncOperation", "http://"+r.Host+"/operations/delete")
						w.WriteHeader(http.StatusAccepted)
						return
					}
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(http.StatusMethodNotAllowed)
				}
			})
			mux.HandleFunc("/operations/delete", func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				calls = append(calls, "POLL")
				handleJSONResponse(t, map[string]any{
					"error": map[string]string{"code": "NoRegisteredProviderFound", "message": "poll failed"},
				}, http.StatusBadRequest)(w, r)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			c := NewResourceClient(newArmOptions(server.URL), nil, nil)
			c.armClientOptions = newClientOptions(server.Client(), server.URL)
			c.armClientOptions.Retry.MaxRetries = -1

			err = c.Delete(t.Context(), id)
			if tt.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantError)
				var resourceError *ResourceError
				require.ErrorAs(t, err, &resourceError)
				var responseError *azcore.ResponseError
				require.True(t, errors.As(err, &responseError))
				require.Equal(t, tt.wantError, responseError.ErrorCode)
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tt.want, calls)
		})
	}
}

func Test_Delete_ARM_TransportFailure(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			mux := http.NewServeMux()
			mux.HandleFunc(ARMProviderPath, handleJSONResponse(t, armresources.Provider{
				ResourceTypes: []*armresources.ProviderResourceType{{
					ResourceType: new("virtualMachines"),
					APIVersions:  []*string{new(ARMAPIVersion), new("2025-08-01")},
				}},
			}, http.StatusOK))
			mux.HandleFunc(ARMResourceID, handleDeleteSuccess())
			server := httptest.NewServer(mux)
			defer server.Close()
			c := NewResourceClient(newArmOptions(server.URL), nil, nil)
			c.armClientOptions = newClientOptions(server.Client(), server.URL)
			c.armClientOptions.Retry.MaxRetries = -1
			failure := errors.New("connection lost")
			var calls []string
			c.armClientOptions.Transport = armTestTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == ARMResourceID {
					calls = append(calls, r.Method+" "+r.URL.Query().Get("api-version"))
					if r.Method == method {
						return nil, failure
					}
				}
				return server.Client().Do(r)
			})

			err := c.Delete(t.Context(), ARMResourceID)
			require.ErrorIs(t, err, failure)
			expected := []string{"GET " + ARMAPIVersion}
			if method == http.MethodDelete {
				expected = append(expected, "DELETE "+ARMAPIVersion)
			}
			require.Equal(t, expected, calls)
		})
	}
}

type armTestTransport func(*http.Request) (*http.Response, error)

func (f armTestTransport) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}
