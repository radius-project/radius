/*
Copyright 2023 The Radius Authors.

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

package applications

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	azpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/testutil"
	"github.com/stretchr/testify/require"
)

func Test_isResourceInEnvironment(t *testing.T) {
	id := "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/Applications/myapp"

	type args struct {
		resource        generated.GenericResource
		environmentName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "resource is in environment",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]any{
						"environment": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0",
					},
				},
				environmentName: "env0",
			},
			want: true,
		},
		{
			name: "resource is not in environment",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]any{
						"environment": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0",
					},
				},
				environmentName: "env",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isResourceInEnvironment(tt.args.resource, tt.args.environmentName))
		})
	}
}

func Test_isResourceInApplication(t *testing.T) {
	id := "/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/Applications/myapp"

	type args struct {
		resource        generated.GenericResource
		applicationName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "resource is in application",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]any{
						"application": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/myapp",
					},
				},
				applicationName: "myapp",
			},
			want: true,
		},
		{
			name: "resource is not in application",
			args: args{
				resource: generated.GenericResource{
					ID: &id,
					Properties: map[string]any{
						"application": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/applications/myapp",
					},
				},
				applicationName: "myapp2",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isResourceInApplication(tt.args.resource, tt.args.applicationName))
		})
	}
}

func Test_computeGraph(t *testing.T) {
	tests := []struct {
		name                string
		appResourceDataFile string
		envResourceDataFile string
		expectedDataFile    string
	}{
		{
			name:                "direct route",
			appResourceDataFile: "graph-app-directroute-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-directroute-out.json",
		},
		{
			name:                "with gateway route",
			appResourceDataFile: "graph-app-gw-in.json",
			envResourceDataFile: "",
			expectedDataFile:    "graph-app-gw-out.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appResource := []generated.GenericResource{}
			envResource := []generated.GenericResource{}

			if tt.appResourceDataFile != "" {
				testutil.MustUnmarshalFromFile(tt.appResourceDataFile, &appResource)
			}

			if tt.envResourceDataFile != "" {
				testutil.MustUnmarshalFromFile(tt.envResourceDataFile, &envResource)
			}

			expected := []*corerpv20231001preview.ApplicationGraphResource{}
			testutil.MustUnmarshalFromFile(tt.expectedDataFile, &expected)

			got := computeGraph(appResource, envResource, "")
			require.ElementsMatch(t, expected, got.Resources)
		})
	}
}

func TestFindSourceResource(t *testing.T) {
	tests := []struct {
		name             string
		source           string
		resourceDataFile string

		parsedSource string
		wantErr      error
	}{
		{
			name:             "valid source ID",
			source:           "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Datastores/sqlDatabases/sql-db",
			wantErr:          nil,
		},
		{
			name:             "invalid source",
			source:           "invalid",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "invalid",
			wantErr:          ErrInvalidSource,
		},
		{
			name:             "direct route without scheme",
			source:           "backendapp:8080",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backendapp",
			wantErr:          nil,
		},
		{
			name:             "direct route with scheme",
			source:           "http://backendapp:8080",
			resourceDataFile: "graph-app-directroute-in.json",
			parsedSource:     "/planes/radius/local/resourcegroups/default/providers/Applications.Core/containers/backendapp",
			wantErr:          nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resources := []generated.GenericResource{}
			testutil.MustUnmarshalFromFile(tc.resourceDataFile, &resources)
			parsedSource, err := findSourceResource(tc.source, resources)
			require.Equal(t, tc.parsedSource, parsedSource)
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

// Test_getAPIVersionForResourceType_Validation tests the resource type format validation
func Test_getAPIVersionForResourceType_Validation(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		wantErr      bool
		errContains  string
	}{
		{
			name:         "returns error for invalid resource type format - no slash",
			resourceType: "InvalidFormat",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for invalid resource type format - too many slashes",
			resourceType: "Test.Resources/postgres/extra",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for empty resource type",
			resourceType: "",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
		{
			name:         "returns error for only slash",
			resourceType: "/",
			wantErr:      true,
			errContains:  "invalid resource type format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// For validation tests, we just need to check the parsing logic
			// We'll use a nil clientOptions since validation happens first
			_, err := getAPIVersionForResourceType(context.Background(), tt.resourceType, nil)

			// Verify results
			require.Error(t, err)
			if tt.errContains != "" {
				require.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}

func Test_getResourceTypeSpecificProperties(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want map[string]any
	}{
		{
			name: "nil in nil out",
			in:   nil,
			want: nil,
		},
		{
			name: "drops only-captured fields and returns nil",
			in: map[string]any{
				"provisioningState": "Succeeded",
				"connections":       map[string]any{"db": map[string]any{"source": "x"}},
				"status":            map[string]any{"outputResources": []any{map[string]any{"id": "/a/b"}}, "phrase": "All good"},
			},
			want: nil,
		},
		{
			name: "drops status entirely while keeping other fields",
			in: map[string]any{
				"status": map[string]any{
					"outputResources": []any{map[string]any{"id": "/a/b"}},
					"phrase":          "All good",
				},
				"application": "/planes/radius/local/.../applications/myapp",
				"image":       "magpie:latest",
			},
			want: map[string]any{
				"application": "/planes/radius/local/.../applications/myapp",
				"image":       "magpie:latest",
			},
		},
		{
			name: "preserves routes alongside other fields",
			in: map[string]any{
				"provisioningState": "Succeeded",
				"connections":       map[string]any{"db": map[string]any{"source": "x"}},
				"routes": []any{map[string]any{
					"path":        "/api",
					"destination": "http://backend:8080",
				}},
				"hostname": "example.com",
			},
			want: map[string]any{
				"routes": []any{map[string]any{
					"path":        "/api",
					"destination": "http://backend:8080",
				}},
				"hostname": "example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getResourceTypeSpecificProperties(tt.in)
			require.Equal(t, tt.want, got)
		})
	}
}

func Test_azurePortalURL(t *testing.T) {
	const tenantID = "11111111-1111-1111-1111-111111111111"
	const azureStorageID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"
	const azureSubscriptionScopedID = "/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Resources/tags/default"
	const ucpQualifiedID = "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/mycontainer"
	const kubernetesUCPID = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/foo"

	tests := []struct {
		name     string
		id       resources.ID
		tenantID string
		want     string
	}{
		{
			name:     "empty tenant returns empty URL",
			id:       resources.MustParse(azureStorageID),
			tenantID: "",
			want:     "",
		},
		{
			name:     "UCP-qualified Radius ID is not an Azure resource",
			id:       resources.MustParse(ucpQualifiedID),
			tenantID: tenantID,
			want:     "",
		},
		{
			name:     "UCP-qualified Kubernetes ID is not an Azure resource",
			id:       resources.MustParse(kubernetesUCPID),
			tenantID: tenantID,
			want:     "",
		},
		{
			name:     "zero-value ID has no scope segments",
			id:       resources.ID{},
			tenantID: tenantID,
			want:     "",
		},
		{
			name:     "Azure ARM ID with subscription and resource group produces portal URL",
			id:       resources.MustParse(azureStorageID),
			tenantID: tenantID,
			want:     "https://portal.azure.com/#@" + tenantID + "/resource" + azureStorageID,
		},
		{
			name:     "Azure ARM ID scoped to a subscription only produces portal URL",
			id:       resources.MustParse(azureSubscriptionScopedID),
			tenantID: tenantID,
			want:     "https://portal.azure.com/#@" + tenantID + "/resource" + azureSubscriptionScopedID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, azurePortalURL(tt.id, tt.tenantID))
		})
	}
}

func Test_outputResourceEntryFromID(t *testing.T) {
	const tenantID = "11111111-1111-1111-1111-111111111111"
	const azureStorageID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"
	const ucpQualifiedID = "/planes/radius/local/resourceGroups/default/providers/Applications.Core/containers/mycontainer"

	tests := []struct {
		name          string
		id            string
		tenantID      string
		wantName      string
		wantType      string
		wantPortalURL string // empty means PortalURL must be nil
	}{
		{
			name:          "Azure ID with tenant sets PortalURL",
			id:            azureStorageID,
			tenantID:      tenantID,
			wantName:      "mystorage",
			wantType:      "Microsoft.Storage/storageAccounts",
			wantPortalURL: "https://portal.azure.com/#@" + tenantID + "/resource" + azureStorageID,
		},
		{
			name:          "Azure ID without tenant omits PortalURL",
			id:            azureStorageID,
			tenantID:      "",
			wantName:      "mystorage",
			wantType:      "Microsoft.Storage/storageAccounts",
			wantPortalURL: "",
		},
		{
			name:          "UCP-qualified ID with tenant omits PortalURL",
			id:            ucpQualifiedID,
			tenantID:      tenantID,
			wantName:      "mycontainer",
			wantType:      "Applications.Core/containers",
			wantPortalURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := resources.Parse(tt.id)
			require.NoError(t, err)

			entry := outputResourceEntryFromID(id, tt.tenantID)

			require.NotNil(t, entry.ID)
			require.Equal(t, tt.id, *entry.ID)
			require.NotNil(t, entry.Name)
			require.Equal(t, tt.wantName, *entry.Name)
			require.NotNil(t, entry.Type)
			require.Equal(t, tt.wantType, *entry.Type)

			if tt.wantPortalURL == "" {
				require.Nil(t, entry.PortalURL)
			} else {
				require.NotNil(t, entry.PortalURL)
				require.Equal(t, tt.wantPortalURL, *entry.PortalURL)
			}
		})
	}
}

func Test_outputResourcesFromAPIData(t *testing.T) {
	const tenantID = "11111111-1111-1111-1111-111111111111"
	const azureStorageID = "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorage"
	const kubernetesUCPID = "/planes/kubernetes/local/namespaces/default/providers/apps/Deployment/foo"

	makeResource := func(properties map[string]any) generated.GenericResource {
		return generated.GenericResource{Properties: properties}
	}

	tests := []struct {
		name              string
		resource          generated.GenericResource
		tenantID          string
		wantIDsInOrder    []string
		wantPortalURLByID map[string]string // empty string / missing key means PortalURL must be nil
	}{
		{
			name:           "missing status returns empty slice",
			resource:       makeResource(map[string]any{}),
			tenantID:       tenantID,
			wantIDsInOrder: nil,
		},
		{
			name:           "status without outputResources returns empty slice",
			resource:       makeResource(map[string]any{"status": map[string]any{}}),
			tenantID:       tenantID,
			wantIDsInOrder: nil,
		},
		{
			name: "empty outputResources returns empty slice",
			resource: makeResource(map[string]any{
				"status": map[string]any{"outputResources": []any{}},
			}),
			tenantID:       tenantID,
			wantIDsInOrder: nil,
		},
		{
			name: "entries without id are skipped",
			resource: makeResource(map[string]any{
				"status": map[string]any{"outputResources": []any{
					map[string]any{"id": ""},
					map[string]any{"id": azureStorageID},
				}},
			}),
			tenantID:       tenantID,
			wantIDsInOrder: []string{azureStorageID},
			wantPortalURLByID: map[string]string{
				azureStorageID: "https://portal.azure.com/#@" + tenantID + "/resource" + azureStorageID,
			},
		},
		{
			name: "Azure and UCP IDs coexist; only Azure IDs get PortalURL",
			resource: makeResource(map[string]any{
				"status": map[string]any{"outputResources": []any{
					map[string]any{"id": azureStorageID},
					map[string]any{"id": kubernetesUCPID},
				}},
			}),
			tenantID: tenantID,
			// Sort order is by Type then Name then ID. Azure Type="Microsoft.Storage/storageAccounts",
			// Kubernetes Type="apps/Deployment". 'M' (0x4d) < 'a' (0x61), so the Azure entry sorts first.
			wantIDsInOrder: []string{azureStorageID, kubernetesUCPID},
			wantPortalURLByID: map[string]string{
				azureStorageID: "https://portal.azure.com/#@" + tenantID + "/resource" + azureStorageID,
			},
		},
		{
			name: "empty tenant leaves PortalURL nil for all entries",
			resource: makeResource(map[string]any{
				"status": map[string]any{"outputResources": []any{
					map[string]any{"id": azureStorageID},
				}},
			}),
			tenantID:       "",
			wantIDsInOrder: []string{azureStorageID},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := outputResourcesFromAPIData(tt.resource, tt.tenantID)

			require.Len(t, got, len(tt.wantIDsInOrder))
			for i, wantID := range tt.wantIDsInOrder {
				require.NotNil(t, got[i].ID)
				require.Equal(t, wantID, *got[i].ID, "entry %d ID", i)

				wantURL, hasURL := tt.wantPortalURLByID[wantID]
				if hasURL && wantURL != "" {
					require.NotNil(t, got[i].PortalURL, "entry %d expected PortalURL", i)
					require.Equal(t, wantURL, *got[i].PortalURL)
				} else {
					require.Nil(t, got[i].PortalURL, "entry %d expected no PortalURL", i)
				}
			}
		})
	}
}

func Test_azureTenantID(t *testing.T) {
	const spTenant = "22222222-2222-2222-2222-222222222222"
	const wiTenant = "33333333-3333-3333-3333-333333333333"

	// Body helpers matching the AzureCredentialResource wire format at
	// GET /planes/azure/azurecloud/providers/System.Azure/credentials/default.
	servicePrincipalBody := `{
	  "id": "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
	  "name": "default",
	  "type": "System.Azure/credentials",
	  "location": "global",
	  "properties": {
	    "kind": "ServicePrincipal",
	    "tenantId": "` + spTenant + `",
	    "clientId": "client-id",
	    "storage": {"kind": "Internal"}
	  }
	}`
	workloadIdentityBody := `{
	  "id": "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
	  "name": "default",
	  "type": "System.Azure/credentials",
	  "location": "global",
	  "properties": {
	    "kind": "WorkloadIdentity",
	    "tenantId": "` + wiTenant + `",
	    "clientId": "client-id",
	    "storage": {"kind": "Internal"}
	  }
	}`
	unknownKindBody := `{
	  "id": "/planes/azure/azurecloud/providers/System.Azure/credentials/default",
	  "name": "default",
	  "type": "System.Azure/credentials",
	  "location": "global",
	  "properties": {"kind": "Unknown"}
	}`

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{
			name: "service principal credential returns tenant ID",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(servicePrincipalBody))
			},
			want: spTenant,
		},
		{
			name: "workload identity credential returns tenant ID",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(workloadIdentityBody))
			},
			want: wiTenant,
		},
		{
			name: "unknown credential kind returns empty tenant",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(unknownKindBody))
			},
			want: "",
		},
		{
			name: "not found response returns empty tenant",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			want: "",
		},
		{
			name: "server error response returns empty tenant",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			t.Cleanup(server.Close)

			opts := &policy.ClientOptions{
				ClientOptions: azpolicy.ClientOptions{
					Transport: server.Client(),
					Cloud: cloud.Configuration{
						Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
							cloud.ResourceManager: {
								Endpoint: server.URL,
								Audience: "https://management.core.windows.net",
							},
						},
					},
					InsecureAllowCredentialWithHTTP: true,
					Retry: azpolicy.RetryOptions{
						MaxRetries: -1, // disable retries so 5xx responses don't slow the test
					},
				},
			}

			got := azureTenantID(context.Background(), opts)
			require.Equal(t, tt.want, got)
		})
	}
}
