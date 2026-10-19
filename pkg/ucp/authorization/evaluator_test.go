// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

const (
	plane = "/planes/radius/local"
	teamA = plane + "/resourceGroups/team-a"
	teamB = plane + "/resourceGroups/team-b"
	appA  = teamA + "/providers/Radius.Core/applications/app"
	envA  = teamA + "/providers/Radius.Core/environments/staging"
)

func testPrincipal(typ api.PrincipalType, issuer, subject string) api.Principal {
	return api.Principal{Type: to.Ptr(typ), Issuer: to.Ptr(issuer), Subject: to.Ptr(subject)}
}

func testIdentity() Identity {
	return Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "alice")}
}

func testAssignment(name, role, scope string, principal api.Principal) api.RoleAssignmentResource {
	idScope := scope
	if scope == "/" {
		idScope = ""
	}
	return api.RoleAssignmentResource{
		ID: to.Ptr(idScope + "/providers/Radius.Core/roleAssignments/" + name),
		Properties: &api.RoleAssignmentProperties{
			RoleDefinitionID: to.Ptr(role), Principal: &principal, Scope: to.Ptr(scope),
		},
	}
}

func TestRoleBoundaries(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	for _, tt := range []struct {
		role, scope, permission, target string
		allowed                         bool
	}{
		{"application-developer", teamA, "Radius.Core/applications/write", appA, true},
		{"application-developer", teamA, "Radius.Core/applications/write", teamB + "/providers/Radius.Core/applications/app", false},
		{"application-developer", appA, "Radius.Core/applications/write", appA, true},
		{"application-developer", appA, "Radius.Core/applications/write", appA + "-other", false},
		{"application-developer", appA, "Radius.Core/applications/write", teamA + "/providers/Radius.Compute/containers/container", false},
		{"application-developer", teamA, "Radius.Core/environments/use/action", envA, false},
		{"application-developer", teamA, "Radius.Core/environments/write", envA, false},
		{"application-developer", teamA, "Radius.Core/recipePacks/write", teamA + "/providers/Radius.Core/recipePacks/pack", false},
		{"application-developer", teamA, "Radius.Core/bicepSettings/write", teamA + "/providers/Radius.Core/bicepSettings/default", false},
		{"application-developer", teamA, "Radius.Core/terraformSettings/delete", teamA + "/providers/Radius.Core/terraformSettings/default", false},
		{"application-developer", plane, "System.Azure/credentials/use/action", "/planes/azure/local", false},
		{"application-developer", teamA, "Microsoft.Resources/deployments/write", teamA, true},
		{"application-developer", teamA, "Applications.Core/extenders/listSecrets/action", teamA, false},
		{"environment-deployer", envA, "Radius.Core/environments/use/action", envA, true},
		{"environment-deployer", envA, "Radius.Core/environments/use/action", envA + "-prod", false},
		{"environment-deployer", envA, "Radius.Core/recipePacks/use/action", teamA + "/providers/Radius.Core/recipePacks/pack", false},
		{"platform-administrator", "/", "System.Azure/credentials/use/action", "/planes/azure/local", true},
		{"platform-administrator", "/", "System.Azure/credentials/write", "/planes/azure/local", false},
		{"platform-administrator", "/", "Radius.Core/roleAssignments/write", teamA, false},
		{"platform-administrator", "/", "Radius.Core/recipePacks/write", teamA, false},
		{"reader", "/", "Radius.Core/applications/read", appA, true},
		{"reader", "/", "Radius.Core/applications/getGraph/action", appA, true},
		{"reader", "/", "Applications.Core/environments/getMetadata/action", envA, true},
		{"reader", "/", "System.Azure/credentials/read", "/planes/azure/local", false},
		{"reader", "/", "Radius.Core/roleAssignments/read", "/", false},
		{"reader", "/", "Applications.Core/secretStores/listSecrets/action", teamA, false},
		{"auditor", "/", "Radius.Core/accessChanges/read", "/", true},
		{"auditor", "/", "Radius.Core/applications/read", appA, false},
		{"access-administrator", "/", "Radius.Core/roleAssignments/write", teamA, true},
		{"access-administrator", "/", "Radius.Core/authorizationSettings/write", "/", false},
		{"access-administrator", "/", "Radius.Core/applications/write", appA, false},
		{"recipe-pack-administrator", teamA, "Radius.Core/recipePacks/use/action", teamA, true},
		{"recipe-pack-administrator", teamA, "Radius.Core/environments/use/action", envA, false},
		{"resource-type-administrator", plane, JoinPermissionSet, plane, true},
		{"radius-administrator", "/", "System.Azure/credentials/write", "/planes/azure/local", true},
		{"radius-administrator", plane, "Radius.Core/applications/write", "/planes/radius/other/resourceGroups/team-a", false},
	} {
		t.Run(tt.role+"/"+tt.permission+"/"+tt.target, func(t *testing.T) {
			identity := testIdentity()
			evaluator, err := NewEvaluator(catalog, nil, []api.RoleAssignmentResource{testAssignment("grant", builtInID(tt.role), tt.scope, identity.Principal)})
			require.NoError(t, err)
			result, err := evaluator.Evaluate(identity, []Check{{tt.permission, tt.target}})
			require.NoError(t, err)
			require.Equal(t, tt.allowed, result[0].Allowed)
		})
	}
}

func TestIdentityAndGroups(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	group := testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "developers")
	user := testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "developers")
	evaluator, err := NewEvaluator(catalog, nil, []api.RoleAssignmentResource{testAssignment("group", builtInID("application-developer"), teamA, group)})
	require.NoError(t, err)
	for _, tt := range []struct {
		name     string
		identity Identity
		allow    bool
		fail     bool
	}{
		{"verifiedGroup", Identity{testIdentity().Principal, []api.Principal{group}}, true, false},
		{"groupDirect", Identity{group, nil}, true, false},
		{"noGroups", testIdentity(), false, false},
		{"sameNameDifferentType", Identity{user, nil}, false, false},
		{"differentIssuer", Identity{testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-b", "developers"), nil}, false, false},
		{"groupIssuerMismatch", Identity{testPrincipal(api.PrincipalTypeUser, "other", "alice"), []api.Principal{group}}, false, true},
		{"groupMembershipNotEnumerable", Identity{group, []api.Principal{group}}, false, true},
		{"anonymous", Identity{testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "system:anonymous"), nil}, false, true},
		{"unauthenticatedGroup", Identity{testIdentity().Principal, []api.Principal{testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "system:unauthenticated")}}, false, true},
		{"empty", Identity{}, false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.Evaluate(tt.identity, []Check{{"Radius.Core/applications/write", appA}})
			if tt.fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.allow, result[0].Allowed)
			}
		})
	}
}

func TestApplicationResourceOptIn(t *testing.T) {
	for _, optIn := range []bool{false, true} {
		t.Run(map[bool]string{false: "unregisteredSet", true: "optedIn"}[optIn], func(t *testing.T) {
			registration := Registration{Type: "Team.Data/widgets", Actions: []string{"restart"}}
			if optIn {
				registration.PermissionSets = []string{ApplicationResources}
			}
			catalog, err := NewCatalog([]Registration{registration})
			require.NoError(t, err)
			customID := teamA + "/providers/Radius.Core/roleDefinitions/custom"
			custom := api.RoleDefinitionResource{ID: to.Ptr(customID), Properties: &api.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Team.Data/widgets/read")}}
			for _, role := range []string{"application-developer", "radius-administrator", "reader", customID} {
				roleID := builtInID(role)
				if role == customID {
					roleID = customID
				}
				evaluator, err := NewEvaluator(catalog, []api.RoleDefinitionResource{custom}, []api.RoleAssignmentResource{testAssignment("grant", roleID, teamA, testIdentity().Principal)})
				require.NoError(t, err)
				for _, op := range []string{"read", "write", "delete", "connect/action", "restart/action"} {
					result, err := evaluator.Evaluate(testIdentity(), []Check{{"Team.Data/widgets/" + op, teamA + "/providers/Team.Data/widgets/test"}})
					require.NoError(t, err)
					want := role == "radius-administrator" || role == "application-developer" && optIn && op != "restart/action" || role == customID && op == "read"
					require.Equal(t, want, result[0].Allowed, "%s %s", role, op)
				}
			}
		})
	}
}

func TestPolicyValidation(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	customID := teamA + "/providers/Radius.Core/roleDefinitions/custom"
	custom := api.RoleDefinitionResource{ID: to.Ptr(customID), Properties: &api.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/applications/read"), AssignableScopes: to.SliceOfPtrs(teamA)}}
	for _, tt := range []struct {
		name, scope, role string
	}{
		{"outsideAssignableScope", teamB, customID},
		{"resourceTypeAdminAtGroup", teamA, builtInID("resource-type-administrator")},
		{"applicationDeveloperAtRoot", "/", builtInID("application-developer")},
		{"environmentDeployerAtRoot", "/", builtInID("environment-deployer")},
		{"unsupportedResource", teamA + "/providers/Radius.Compute/containers/test", customID},
		{"unknownRole", teamA, builtInID("unknown")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEvaluator(catalog, []api.RoleDefinitionResource{custom}, []api.RoleAssignmentResource{testAssignment("grant", tt.role, tt.scope, testIdentity().Principal)})
			require.Error(t, err)
		})
	}
	for _, permission := range []string{"*", "Radius.Core/newType/read"} {
		definition := custom
		properties := *custom.Properties
		properties.Permissions = to.SliceOfPtrs(permission)
		definition.Properties = &properties
		_, err := NewEvaluator(catalog, []api.RoleDefinitionResource{definition}, nil)
		require.ErrorContains(t, err, "UnknownPermission")
	}
	spoof := custom
	properties := *custom.Properties
	properties.RoleType = to.Ptr(api.RoleTypeBuiltIn)
	spoof.Properties = &properties
	_, err = NewEvaluator(catalog, []api.RoleDefinitionResource{spoof}, nil)
	require.ErrorContains(t, err, "BuiltInRoleImmutable")
	assignment := testAssignment("grant", customID, teamA, testIdentity().Principal)
	assignment.Properties.Scope = to.Ptr(teamB)
	_, err = NewEvaluator(catalog, []api.RoleDefinitionResource{custom}, []api.RoleAssignmentResource{assignment})
	require.ErrorContains(t, err, "scope does not match")
}

func TestScopeContains(t *testing.T) {
	for _, tt := range []struct {
		parent, child string
		want          bool
	}{
		{"/", appA, true}, {plane, appA, true}, {teamA, appA, true},
		{teamA, teamB, false}, {appA, appA + "-sibling", false},
		{envA, appA, false}, {plane, "/planes/radius/other", false},
		{plane, "/planes/azure/local", false}, {"/PLANES/RADIUS/LOCAL", appA, true},
	} {
		got, err := ScopeContains(tt.parent, tt.child)
		require.NoError(t, err)
		require.Equal(t, tt.want, got)
	}

	for _, invalid := range []string{"", "relative", plane + "/../other", plane + "//resourceGroups/team-a", plane + "/resourceGroups/%2f", "/subscriptions/sub", "/planes/unknown/local", teamA + "/providers/Radius.Core/applications"} {
		_, err := ScopeContains("/", invalid)
		require.Error(t, err, invalid)
	}
}

func TestCustomRoleAtInstallation(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	id := "/providers/Radius.Core/roleDefinitions/custom-reader"
	definition := api.RoleDefinitionResource{
		ID: to.Ptr(id), Properties: &api.RoleDefinitionProperties{
			Permissions: to.SliceOfPtrs("Radius.Core/applications/read"),
		},
	}
	identity := testIdentity()
	assignment := testAssignment("custom", id, "/", identity.Principal)
	evaluator, err := NewEvaluator(catalog, []api.RoleDefinitionResource{definition}, []api.RoleAssignmentResource{assignment})
	require.NoError(t, err)
	result, err := evaluator.Evaluate(identity, []Check{
		{"Radius.Core/applications/read", appA},
		{"Radius.Core/applications/write", appA},
	})
	require.NoError(t, err)
	require.True(t, result[0].Allowed)
	require.False(t, result[1].Allowed)
	definition.Properties.AssignableScopes = to.SliceOfPtrs(teamA)
	_, err = NewEvaluator(catalog, []api.RoleDefinitionResource{definition}, []api.RoleAssignmentResource{assignment})
	require.ErrorContains(t, err, "ScopeNotAssignable")
}

func TestEvaluatorSnapshotAndErrors(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	identity := testIdentity()
	assignment := testAssignment("reader", builtInID("reader"), teamA, identity.Principal)
	evaluator, err := NewEvaluator(catalog, nil, []api.RoleAssignmentResource{assignment})
	require.NoError(t, err)
	*assignment.Properties.Principal.Subject = "mallory"
	*assignment.Properties.Scope = teamB
	// The identity shares the original principal's pointer fields with the input.
	identity = testIdentity()
	result, err := evaluator.Evaluate(identity, []Check{{"Radius.Core/applications/read", appA}})
	require.NoError(t, err)
	require.True(t, result[0].Allowed)
	for _, checks := range [][]Check{nil, {{"unknown", appA}}, {{"Radius.Core/applications/read", "invalid"}}} {
		_, err := evaluator.Evaluate(identity, checks)
		require.Error(t, err)
	}
}
