// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func customRole(id string, permissions ...string) api.RoleDefinitionResource {
	return api.RoleDefinitionResource{ID: to.Ptr(id), Properties: &api.RoleDefinitionProperties{
		RoleType: to.Ptr(api.RoleTypeCustom), Permissions: to.SliceOfPtrs(permissions...),
	}}
}

func TestPolicySafety(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	roleID := teamA + "/providers/Radius.Core/roleDefinitions/operator"
	for _, scenario := range []struct {
		name string
		edit func(*Policy, *PolicyActor, *PolicyMutation)
		code string
	}{
		{"valid", func(*Policy, *PolicyActor, *PolicyMutation) {}, ""},
		{"unknown permission", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			m.Target, m.Assignment = roleID, nil
			m.Role = &api.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/notRegistered/write")}
		}, "UnknownPermission"},
		{"builtin update", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			m.Target, m.Assignment = builtInID("reader"), nil
			m.Role = &api.RoleDefinitionProperties{}
		}, "BuiltInRoleImmutable"},
		{"builtin delete", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = PolicyMutation{Target: builtInID("reader"), Delete: true}
		}, "BuiltInRoleImmutable"},
		{"role type spoof", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = PolicyMutation{Target: roleID, Role: &api.RoleDefinitionProperties{RoleType: to.Ptr(api.RoleTypeBuiltIn)}}
		}, "BuiltInRoleImmutable"},
		{"last administrator", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.Assignments = p.Assignments[:1]
			*m = PolicyMutation{Target: *p.Assignments[0].ID, Delete: true}
		}, "LastAdministratorProtected"},
		{"bootstrap does not count", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.Assignments[1].Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
			*m = PolicyMutation{Target: *p.Assignments[0].ID, Delete: true}
		}, "LastAdministratorProtected"},
		{"workload does not count", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.Assignments[1].Properties.Principal.Type = to.Ptr(api.PrincipalTypeWorkload)
			*m = PolicyMutation{Target: *p.Assignments[0].ID, Delete: true}
		}, "LastAdministratorProtected"},
		{"bootstrap ownership", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.Assignments[1].Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
			*m = PolicyMutation{Target: *p.Assignments[1].ID, Delete: true}
		}, "ManagedByConflict"},
		{"bootstrap spoof", func(_ *Policy, a *PolicyActor, _ *PolicyMutation) {
			a.Management.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
		}, "ManagedByConflict"},
		{"assignment role immutable", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = assignmentMutation("alpha", builtInID("reader"), "/", *p.Assignments[0].Properties.Principal.Subject)
		}, "RoleAssignmentImmutable"},
		{"assignment principal immutable", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = assignmentMutation("alpha", builtInID("radius-administrator"), "/", "mallory")
		}, "RoleAssignmentImmutable"},
		{"duplicate tuple", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = assignmentMutation("duplicate", builtInID("radius-administrator"), "/", *p.Assignments[0].Properties.Principal.Subject)
		}, "RoleAssignmentImmutable"},
		{"unauthorized", func(_ *Policy, a *PolicyActor, _ *PolicyMutation) {
			a.Identity.Principal.Subject = to.Ptr("mallory")
			a.Identity.Groups = nil
		}, "AuthorizationFailed"},
		{"unknown role", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			m.Assignment.RoleDefinitionID = to.Ptr(roleID + "-absent")
		}, "unknown role"},
		{"assignment outside allowed scope", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.CustomRoles = append(p.CustomRoles, customRole(roleID, "Radius.Core/applications/read"))
			*m = assignmentMutation("operator", roleID, teamB, "bob")
		}, "ScopeNotAssignable"},
		{"custom installation role", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			id := "/providers/Radius.Core/roleDefinitions/root-custom"
			p.CustomRoles = append(p.CustomRoles, customRole(id, "Radius.Core/applications/read"))
			*m = assignmentMutation("root-custom", id, "/", "bob")
		}, ""},
		{"role in use", func(p *Policy, _ *PolicyActor, m *PolicyMutation) {
			p.CustomRoles = append(p.CustomRoles, customRole(roleID, "Radius.Core/applications/read"))
			a := testAssignment("operator", roleID, teamA, testIdentity().Principal)
			a.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
			p.Assignments = append(p.Assignments, a)
			*m = PolicyMutation{Target: roleID, Delete: true}
		}, "RoleDefinitionInUse"},
		{"settings guard missing", func(_ *Policy, _ *PolicyActor, m *PolicyMutation) {
			*m = PolicyMutation{Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeEnforce)}}
		}, "settings transition guard"},
		{"ownership override put", func(_ *Policy, a *PolicyActor, _ *PolicyMutation) {
			a.TakeOwnership = true
		}, "deletion-only"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			policy, actor, mutation := initialPolicy(), policyActor(), readerMutation()
			scenario.edit(&policy, &actor, &mutation)
			snapshot, err := NewPolicySnapshot(catalog, policy)
			require.NoError(t, err)
			_, _, _, err = applyPolicyMutation(t.Context(), snapshot, actor, mutation, nil)
			if scenario.code == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, scenario.code)
			}
		})
	}
}

func TestPolicySafetyOwnership(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	for _, scenario := range []struct {
		name                 string
		sameSource, takeover bool
		allowed              bool
	}{
		{"same source", true, false, true},
		{"another source", false, false, false},
		{"administrator takeover", false, true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			policy := initialPolicy()
			assignment := testAssignment("gitops", builtInID("reader"), teamA, testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob"))
			assignment.Properties.ManagedBy = &api.AssignmentManagement{
				Kind: to.Ptr(api.AssignmentManagementKindGitOps), Source: to.Ptr("https://example.test/platform"),
			}
			policy.Assignments = append(policy.Assignments, assignment)
			snapshot, err := NewPolicySnapshot(catalog, policy)
			require.NoError(t, err)
			actor := policyActor()
			actor.TakeOwnership = scenario.takeover
			if scenario.sameSource {
				actor.Management = *assignment.Properties.ManagedBy
			}
			_, _, impact, err := applyPolicyMutation(t.Context(), snapshot, actor, PolicyMutation{Target: *assignment.ID, Delete: true}, nil)
			if scenario.allowed {
				require.NoError(t, err)
				require.Equal(t, scenario.takeover, impact)
			} else {
				require.ErrorContains(t, err, "ManagedByConflict")
			}
		})
	}
}

func TestPolicySafetyDelegation(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	for _, scenario := range []string{"grant missing permission", "grow role", "expand scopes", "grant held permission", "remove unheld permission", "access administrator grants admin"} {
		t.Run(scenario, func(t *testing.T) {
			policy, actor := initialPolicy(), policyActor()
			actor.Identity = Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "bob")}
			id := teamA + "/providers/Radius.Core/roleDefinitions/delegator"
			permissions := []string{"Radius.Core/roleAssignments/write", "Radius.Core/roleDefinitions/write", "Radius.Core/applications/read"}
			policy.CustomRoles = append(policy.CustomRoles, customRole(id, permissions...))
			grant := testAssignment("delegator", id, teamA, actor.Identity.Principal)
			policy.Assignments = append(policy.Assignments, grant)
			targetRole := teamA + "/providers/Radius.Core/roleDefinitions/target"
			policy.CustomRoles = append(policy.CustomRoles, customRole(targetRole, "Radius.Core/applications/read"))
			mutation := assignmentMutation("new", builtInID("reader"), teamA, "charlie")
			expected := "PrivilegeEscalationDenied"
			switch scenario {
			case "grow role":
				mutation = PolicyMutation{Target: targetRole, Role: customRole(targetRole, "Radius.Core/applications/write").Properties}
			case "expand scopes":
				mutation = PolicyMutation{Target: targetRole, Role: customRole(targetRole, "Radius.Core/applications/read").Properties}
				mutation.Role.AssignableScopes = to.SliceOfPtrs(teamA, teamB)
				expected = "AuthorizationFailed"
			case "grant held permission":
				mutation.Assignment.RoleDefinitionID = to.Ptr(targetRole)
				expected = ""
			case "remove unheld permission":
				policy.CustomRoles[len(policy.CustomRoles)-1].Properties.Permissions =
					to.SliceOfPtrs("Radius.Core/applications/read", "Radius.Core/applications/write")
				mutation = PolicyMutation{Target: targetRole, Role: customRole(targetRole, "Radius.Core/applications/read").Properties}
				expected = ""
			case "access administrator grants admin":
				policy.Assignments[len(policy.Assignments)-1] = testAssignment("delegator", builtInID("access-administrator"), "/", actor.Identity.Principal)
				mutation = assignmentMutation("admin-charlie", builtInID("radius-administrator"), "/", "charlie")
				expected = ""
			}
			snapshot, err := NewPolicySnapshot(catalog, policy)
			require.NoError(t, err)
			_, _, highImpact, err := applyPolicyMutation(t.Context(), snapshot, actor, mutation, nil)
			if expected != "" {
				require.ErrorContains(t, err, expected)
			} else {
				require.NoError(t, err)
				require.Equal(t, scenario == "access administrator grants admin", highImpact)
			}
		})
	}
}

func TestPolicySafetyServerFieldsAndBootstrapTransition(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	policy := initialPolicy()
	for _, assignment := range policy.Assignments {
		assignment.Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
	}
	current, err := NewPolicySnapshot(catalog, policy)
	require.NoError(t, err)
	_, _, _, err = applyPolicyMutation(t.Context(), current, policyActor(), readerMutation(), nil)
	require.ErrorContains(t, err, "LastAdministratorProtected")
	mutation := assignmentMutation("permanent", builtInID("radius-administrator"), "/", "charlie")
	mutation.Assignment.Scope = to.Ptr(teamB)
	mutation.Assignment.CreatedBy = to.Ptr("forged")
	mutation.Assignment.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindBootstrap)}
	next, change, impact, err := applyPolicyMutation(t.Context(), current, policyActor(), mutation, nil)
	require.NoError(t, err)
	require.Equal(t, 1, permanentAdministrators(next))
	require.True(t, impact)
	require.Equal(t, "/", *change.After.RoleAssignment.Scope)
	require.Equal(t, api.AssignmentManagementKindImperative, *change.After.RoleAssignment.ManagedBy.Kind)
	require.Equal(t, principalDisplay(policyActor().Identity.Principal), *change.After.RoleAssignment.CreatedBy)
}
