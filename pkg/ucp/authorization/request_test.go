// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/authorization/internal/cataloggen"
	"github.com/stretchr/testify/require"
)

func TestRequestMapping(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
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
		{"GET", "/planes/azure/local/subscriptions/123/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/test", "System.Azure/credentials/use/action", "/planes/azure/local", ResourceRequest},
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

func TestSwaggerOperationsMap(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	var generated cataloggen.Catalog
	require.NoError(t, json.Unmarshal(releaseCatalog, &generated))
	for _, route := range generated.Routes {
		t.Run(route.Method+route.Path, func(t *testing.T) {
			path := sampleSwaggerPath(route.Path)
			mapping, err := catalog.MapRequest(route.Method, path)
			require.NoError(t, err)
			if mapping.Kind != SelfAccessRequest {
				require.NotEmpty(t, mapping.Checks)
			}
			if mapping.Kind != SelfAccessRequest && !strings.HasSuffix(route.Path, "/effectiveAccess") {
				require.Equal(t, strings.ToLower(route.Permission), strings.ToLower(mapping.Checks[0].Permission))
			}
		})
	}
}

func sampleSwaggerPath(path string) string {
	path = strings.ReplaceAll(path, "{rootScope}", "planes/radius/local/resourceGroups/team-a")
	path = strings.ReplaceAll(path, "{scope}", "planes/radius/local/resourceGroups/team-a")
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
