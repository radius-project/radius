// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
	"github.com/stretchr/testify/require"
)

func TestRequestMapping(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	// Paths built the way rad deploy builds them (pkg/cli/deployment/deploy.go).
	deployment := teamA + "/providers/Microsoft.Resources/deployments/rad-deploy"
	for _, tt := range []struct {
		method, path, permission, target string
		kind                             RequestKind
	}{
		{"GET", "/version", "", "", DiscoveryRequest},
		{"GET", "/providers/Radius.Core/whoAmI", "", "", SelfAccessRequest},
		{"GET", "/providers/Applications.Core/operations", "", "", DiscoveryRequest},
		{"GET", "/planes", "System.Resources/planes/read", "/", ListRequest},
		{"GET", "/planes/radius", "System.Radius/planes/read", "/", ListRequest},
		{"GET", teamA, "System.Resources/resourceGroups/read", teamA, ResourceRequest},
		{"GET", plane + "/resourcegroups", "System.Resources/resourceGroups/read", plane, ListRequest},
		{"GET", teamA + "/resources", "System.Resources/resources/read", teamA, MixedResourceListRequest},
		{"GET", appA, "Radius.Core/applications/read", appA, ResourceRequest},
		{"GET", teamA + "/providers/Radius.Core/applications", "Radius.Core/applications/read", teamA, ListRequest},
		{"PATCH", appA, "Radius.Core/applications/write", appA, ResourceRequest},
		{"DELETE", appA, "Radius.Core/applications/delete", appA, ResourceRequest},
		{"POST", appA + "/getGraph", "Radius.Core/applications/getGraph/action", appA, ResourceRequest},
		{"GET", "/providers/Radius.Core/permissions", "Radius.Core/permissions/read", "/", ListRequest},
		{"POST", teamA + "/providers/Radius.Core/checkAccess", "Radius.Core/checkAccess/action", teamA, SelfAccessRequest},
		{"GET", teamA + "/providers/Radius.Core/effectiveAccess", "Radius.Core/roleAssignments/read", teamA, ResourceRequest},
		{"PUT", deployment, "Microsoft.Resources/deployments/write", deployment, ResourceRequest},
		{"GET", deployment, "Microsoft.Resources/deployments/read", deployment, ResourceRequest},
		{"DELETE", deployment, "Microsoft.Resources/deployments/delete", deployment, ResourceRequest},
		{"GET", deployment + "/operations", "Microsoft.Resources/deployments/read", deployment, ResourceRequest},
		{"GET", deployment + "/operations/0123", "Microsoft.Resources/deployments/read", deployment, ResourceRequest},
		{"GET", deployment + "/operationStatuses/0123", "Microsoft.Resources/deployments/read", deployment, ResourceRequest},
		{"GET", teamA + "/providers/Microsoft.Resources/deployments", "Microsoft.Resources/deployments/read", teamA, ListRequest},
		{"GET", plane + "/providers", "System.Resources/providers/read", plane, ListRequest},
		{"GET", plane + "/providers/Team.Data", "System.Resources/providers/read", plane, ResourceRequest},
		{"GET", plane + "/providers/System.Resources/resourceproviders/Team.Data/resourcetypes/widgets/icons/abcd",
			"System.Resources/resourceProviders/resourceTypes/read", plane + "/providers/System.Resources/resourceproviders/Team.Data/resourcetypes/widgets", ResourceRequest},
		{"GET", plane + "/providers/System.Resources/resourceproviders/Team.Data/resourcetypes/widgets/apiversions",
			"System.Resources/resourceProviders/resourceTypes/apiVersions/read", plane + "/providers/System.Resources/resourceproviders/Team.Data/resourcetypes/widgets", ListRequest},
		{"GET", plane + "/providers/System.Resources/locations/global/operationStatuses/123", "", "", OperationStatusRequest},
		{"GET", teamA + "/providers/Applications.Core/locations/global/operationResults/123", "", "", OperationStatusRequest},
		{"GET", "/planes/aws/local/accounts/123/regions/us-west-2/providers/AWS.S3/Bucket/test", "System.AWS/credentials/use/action", "/planes/aws/local", ResourceRequest},
		{"POST", "/planes/aws/local/accounts/123/regions/us-west-2/providers/AWS.S3/Bucket/:put", "System.AWS/credentials/use/action", "/planes/aws/local", ResourceRequest},
		{"GET", "/planes/aws/local/accounts/123/regions/us-west-2/providers/AWS.S3/locations/global/operationStatuses/token", "", "", OperationStatusRequest},
		{"GET", "/planes/aws/local/accounts/123/regions/us-west-2/providers/AWS.S3/locations/global/operationResults/token", "", "", OperationStatusRequest},
		{"PUT", "/planes/aws/local/accounts/123/regions/us-west-2/providers/AWS.S3/locations/global/operationStatuses/token", "System.AWS/credentials/use/action", "/planes/aws/local", ResourceRequest},
		{"GET", "/planes/azure/local/subscriptions/123/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/test", "System.Azure/credentials/use/action", "/planes/azure/local", ResourceRequest},
		{"GET", "/planes/azure/local/providers/Microsoft.Authorization/roleAssignments/grant", "System.Azure/credentials/use/action", "/planes/azure/local", ResourceRequest},
		{"GET", "/planes/azure/local/providers/System.Resources/locations/global/operationStatuses/123", "System.Azure/credentials/use/action", "/planes/azure/local", ResourceRequest},
	} {
		t.Run(tt.method+tt.path, func(t *testing.T) {
			result, err := catalog.MapRequest(tt.method, tt.path)
			require.NoError(t, err)
			require.Equal(t, tt.kind, result.Kind)
			if tt.permission == "" {
				require.Empty(t, result.Checks)
			} else {
				require.Equal(t, []Check{{tt.permission, strings.ToLower(tt.target)}}, result.Checks)
			}
		})
	}
	for _, tt := range []struct{ method, path string }{
		{"HEAD", appA}, {"POST", appA + "/newAction"}, {"PUT", teamA + "/providers/Radius.Core/applications"},
		{"GET", "/planes/radius/local//resourceGroups/team-a"}, {"GET", appA + "/../other"},
		{"GET", appA + "%2f"}, {"GET", "https://host" + appA}, {"GET", appA + "?api-version=2025"},
		{"GET", teamA + "/providers/Unknown.Namespace/types/test"},
	} {
		_, err := catalog.MapRequest(tt.method, tt.path)
		require.Error(t, err, "%s %s", tt.method, tt.path)
	}
}

func TestRequestMappingRouteShortcuts(t *testing.T) {
	t.Parallel()
	catalog, err := NewCatalog([]Registration{{Type: "Foo.Bar/things/icons"}})
	require.NoError(t, err)
	const azure = "/planes/azure/local"
	iconParent := plane + "/providers/System.Resources/resourceproviders/Foo.Bar/resourcetypes/things"
	child := teamA + "/providers/Foo.Bar/things/x/icons/y"
	for _, tt := range []struct {
		name, path, permission, target string
		kind                           RequestKind
	}{
		{"registered icon route", iconParent + "/icons/hash", "System.Resources/resourceProviders/resourceTypes/read", iconParent, ResourceRequest},
		{"case-insensitive icon route", strings.ToUpper(iconParent + "/icons/hash"), "System.Resources/resourceProviders/resourceTypes/read", iconParent, ResourceRequest},
		{"resource-group child named icons", child, "Foo.Bar/things/icons/read", child, ResourceRequest},
		{"Azure Radius.Core proxy", azure + "/providers/Radius.Core/applications/app", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure provider collection proxy", azure + "/providers", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure provider namespace proxy", azure + "/providers/System.Resources", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure System.Resources proxy", azure + "/providers/System.Resources/resourcegroups/rg", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure operation status proxy", azure + "/providers/System.Resources/locations/global/operationStatuses/123", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure icon proxy", strings.Replace(iconParent, plane, azure, 1) + "/icons/hash", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure credential item local", azure + "/providers/System.Azure/credentials/default", "System.Azure/credentials/read", azure + "/providers/System.Azure/credentials/default", ResourceRequest},
		{"Azure credential collection local", azure + "/providers/System.Azure/credentials", "System.Azure/credentials/read", azure, ListRequest},
		{"Azure credential case", azure + "/providers/SYSTEM.AZURE/CREDENTIALS/default", "System.Azure/credentials/read", azure + "/providers/System.Azure/credentials/default", ResourceRequest},
		{"Azure unknown System.Azure type proxy", azure + "/providers/System.Azure/widgets/a", "System.Azure/credentials/use/action", azure, ResourceRequest},
		{"Azure credential descendant proxy", azure + "/providers/System.Azure/credentials/default/children/a", "System.Azure/credentials/use/action", azure, ResourceRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mapping, err := catalog.MapRequest(http.MethodGet, tt.path)
			require.NoError(t, err)
			require.Equal(t, RequestMapping{Kind: tt.kind, Checks: []Check{{tt.permission, strings.ToLower(tt.target)}}}, mapping)
		})
	}
	for _, path := range []string{
		plane + "/providers/Foo.Bar/resourceproviders/rp/resourcetypes/things/icons/hash",
		plane + "/providers/System.Resources/other/rp/resourcetypes/things/icons/hash",
		plane + "/providers/System.Resources/resourceproviders/rp/other/things/icons/hash",
		strings.Replace(iconParent, "/radius/", "/aws/", 1) + "/icons/hash",
	} {
		_, err := catalog.MapRequest(http.MethodGet, path)
		require.Error(t, err, path)
	}
}

func TestCloudPlaneAccessManagement(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	identity := testIdentity()
	for _, plane := range []string{"/planes/azure/local", "/planes/aws/local"} {
		for _, typ := range []string{"roleDefinitions", "roleAssignments"} {
			collection := plane + "/providers/Radius.Core/" + typ
			for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
				t.Run(method+collection, func(t *testing.T) {
					target := collection + "/grant"
					operation := map[string]string{"GET": "read", "PUT": "write", "PATCH": "write", "DELETE": "delete"}[method]
					mapping, err := catalog.MapRequest(method, target)
					require.NoError(t, err)
					require.Equal(t, ResourceRequest, mapping.Kind)
					expected := []Check{{"Radius.Core/" + typ + "/" + operation, strings.ToLower(target)}}
					if plane == "/planes/azure/local" {
						expected = []Check{{"System.Azure/credentials/use/action", plane}}
					}
					require.Equal(t, expected, mapping.Checks)
					if method == "GET" {
						mapping, err = catalog.MapRequest(method, collection)
						require.NoError(t, err)
						if plane == "/planes/azure/local" {
							require.Equal(t, ResourceRequest, mapping.Kind)
							require.Equal(t, expected, mapping.Checks)
						} else {
							require.Equal(t, ListRequest, mapping.Kind)
							require.Equal(t, []Check{{"Radius.Core/" + typ + "/read", plane}}, mapping.Checks)
						}
					}
					for _, role := range []string{"platform-administrator", "access-administrator"} {
						evaluator, err := NewEvaluator(catalog, nil, []api.RoleAssignmentResource{
							testAssignment("grant", builtInID(role), plane, identity.Principal),
						})
						require.NoError(t, err)
						results, err := evaluator.Evaluate(identity, mapping.Checks)
						require.NoError(t, err)
						require.Len(t, results, 1)
						wantAllowed := role == "access-administrator"
						if plane == "/planes/azure/local" {
							wantAllowed = role == "platform-administrator"
						}
						require.Equal(t, wantAllowed, results[0].Allowed, role)
					}
				})
			}
		}
	}
}

func TestSwaggerOperationsMap(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	var generated cataloggen.Catalog
	require.NoError(t, json.Unmarshal(releaseCatalog, &generated))
	for _, route := range generated.Routes {
		scopes := []string{"planes/radius/local/resourceGroups/team-a"}
		if strings.HasPrefix(route.Permission, "Radius.Core/roleDefinitions/") ||
			strings.HasPrefix(route.Permission, "Radius.Core/roleAssignments/") {
			scopes = append(scopes, "planes/radius/local", "planes/azure/local", "planes/aws/local")
		}
		for _, scope := range scopes {
			t.Run(scope+"/"+route.Method+route.Path, func(t *testing.T) {
				path := sampleSwaggerPath(route.Path, scope)
				mapping, err := catalog.MapRequest(route.Method, path)
				require.NoError(t, err)
				if mapping.Kind != SelfAccessRequest {
					require.NotEmpty(t, mapping.Checks)
				}
				if mapping.Kind != SelfAccessRequest {
					expected := route.Permission
					if strings.HasPrefix(path, "/planes/azure/") && strings.Contains(path, "/providers/Radius.Core/role") {
						expected = "System.Azure/credentials/use/action"
					}
					require.Equal(t, strings.ToLower(expected), strings.ToLower(mapping.Checks[0].Permission))
				}
			})
		}
	}
}

func sampleSwaggerPath(path, scope string) string {
	path = strings.ReplaceAll(path, "{rootScope}", scope)
	path = strings.ReplaceAll(path, "{scope}", scope)
	for strings.Contains(path, "{") {
		start := strings.Index(path, "{")
		end := strings.Index(path[start:], "}") + start
		path = path[:start] + "sample" + path[end+1:]
	}
	return path
}

func FuzzMapRequest(f *testing.F) {
	for _, path := range []string{appA, appA + "/getGraph", "/planes/aws/local/accounts/123/regions/east/providers/AWS.S3/Bucket/:put", "/planes/radius/local//bad", "/"} {
		f.Add(http.MethodGet, path)
	}
	catalog, err := NewCatalog(nil)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, method, path string) {
		mapping, err := catalog.MapRequest(method, path)
		if err != nil {
			return
		}
		require.NotEmpty(t, mapping.Kind)
		for _, check := range mapping.Checks {
			require.NoError(t, catalog.ValidatePermissions([]string{check.Permission}))
			require.NotEmpty(t, check.Target)
			_, err := canonicalTarget(check.Target)
			require.NoError(t, err)
		}
	})
}
