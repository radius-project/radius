// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"errors"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	clocktesting "k8s.io/utils/clock/testing"
)

func TestPolicyStoreUnregisteredPermission(t *testing.T) {
	ctx := t.Context()
	db := inmemory.NewClient()
	catalog, err := NewCatalog([]Registration{{Type: "Team.Data/widgets"}})
	require.NoError(t, err)
	writer, err := NewPolicyStore(db, catalog, nil, nil)
	require.NoError(t, err)
	initializer, err := NewPolicyInitializer(writer, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(ctx, initialPolicy()))
	roleID := teamA + "/providers/Radius.Core/roleDefinitions/widgets"
	_, err = writer.Mutate(ctx, policyActor(), PolicyMutation{Target: roleID, Role: customRole(roleID, "Team.Data/widgets/read").Properties})
	require.NoError(t, err)
	assignment := assignmentMutation("widgets", roleID, teamA, "bob")
	_, err = writer.Mutate(ctx, policyActor(), assignment)
	require.NoError(t, err)
	mixedRoleID := roleID + "-mixed"
	_, err = writer.Mutate(ctx, policyActor(), PolicyMutation{
		Target: mixedRoleID, Role: customRole(mixedRoleID, "Team.Data/widgets/read", "Radius.Core/applications/read").Properties,
	})
	require.NoError(t, err)
	mixedAssignment := assignmentMutation("mixed", mixedRoleID, teamA, "charlie")
	_, err = writer.Mutate(ctx, policyActor(), mixedAssignment)
	require.NoError(t, err)

	reader := newTestPolicyStore(t, db, false)
	snapshot, err := reader.LoadPolicy(ctx, -1)
	require.NoError(t, err)
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, "Team.Data/widgets/read", *policy.CustomRoles[0].Properties.Permissions[0])
	cache, err := NewPolicyCache(reader, clocktesting.NewFakeClock(reader.clock.Now()))
	require.NoError(t, err)
	require.NoError(t, cache.Refresh(ctx))
	require.NoError(t, cache.Ready())
	bob := Identity{Principal: *assignment.Assignment.Principal}
	decisions, err := cache.Evaluate(bob, []Check{{"Radius.Core/applications/read", appA}})
	require.NoError(t, err)
	require.False(t, decisions[0].Allowed)
	decisions, err = cache.Evaluate(Identity{Principal: *mixedAssignment.Assignment.Principal}, []Check{{"Radius.Core/applications/read", appA}})
	require.NoError(t, err)
	require.True(t, decisions[0].Allowed, "known grants survive when a sibling permission disappears")
	_, err = cache.Evaluate(bob, []Check{{"Team.Data/widgets/read", teamA + "/providers/Team.Data/widgets/test"}})
	require.Error(t, err)
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: roleID, Role: customRole(roleID, "Team.Data/widgets/read").Properties})
	require.ErrorContains(t, err, "UnknownPermission")
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: mixedAssignment.Target, Delete: true})
	require.NoError(t, err)
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: mixedRoleID, Delete: true})
	require.NoError(t, err, "an unregistered permission must not prevent deleting its role")
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read").Properties})
	require.NoError(t, err)
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: assignment.Target, Delete: true})
	require.NoError(t, err)
	_, err = reader.Mutate(ctx, policyActor(), PolicyMutation{Target: roleID, Delete: true})
	require.NoError(t, err)
}

type bootstrapSettingsGuard struct {
	err        error
	principals []api.Principal
	calls      int
}

func (g *bootstrapSettingsGuard) ValidateSettingsChange(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) error {
	g.calls++
	return g.err
}

func (g *bootstrapSettingsGuard) BootstrapAdministrators(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) ([]api.Principal, error) {
	return g.principals, nil
}

func TestPolicyStoreBootstrapSettings(t *testing.T) {
	for _, scenario := range []struct {
		before, after api.UserAuthorizationMode
	}{
		{api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun},
		{api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce},
		{api.UserAuthorizationModeEnforce, api.UserAuthorizationModeDryRun},
	} {
		t.Run(string(scenario.before)+" to "+string(scenario.after), func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), false)
			policy := initialPolicy()
			policy.Assignments = nil
			policy.Settings.Properties.Mode = &scenario.before
			initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
			require.NoError(t, err)
			require.NoError(t, initializer.Initialize(t.Context(), policy))
			guard := &bootstrapSettingsGuard{principals: []api.Principal{policyActor().Identity.Principal}}
			store.settings = guard
			mutation := PolicyMutation{Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: &scenario.after}}
			_, err = store.Mutate(t.Context(), policyActor(), mutation)
			if scenario.before == api.UserAuthorizationModeEnforce {
				require.ErrorContains(t, err, "AuthorizationFailed")
				require.Zero(t, guard.calls)
				return
			}
			require.NoError(t, err)
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			got, err := snapshot.Policy()
			require.NoError(t, err)
			require.Equal(t, int64(1), got.Generation)
			require.Equal(t, scenario.after, *got.Settings.Properties.Mode)
			require.Len(t, got.Assignments, 1)
			require.Equal(t, api.AssignmentManagementKindBootstrap, *got.Assignments[0].Properties.ManagedBy.Kind)
			event, err := store.AccessChange(t.Context(), 1)
			require.NoError(t, err)
			require.True(t, *event.Properties.HighImpact)
			require.NoError(t, snapshot.allows(policyActor().Identity, "Radius.Core/authorizationSettings/write", settingsID))
			// A repeated settings PUT must not duplicate the trusted assignment.
			_, err = store.Mutate(t.Context(), policyActor(), mutation)
			require.NoError(t, err)
			snapshot, err = store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			got, err = snapshot.Policy()
			require.NoError(t, err)
			require.Equal(t, int64(1), got.Generation)
			require.Len(t, got.Assignments, 1)
		})
	}
}

func TestPolicySettingsBootstrapGuardRequired(t *testing.T) {
	for _, scenario := range []string{"missing guard", "denied", "no administrators", "workload"} {
		t.Run(scenario, func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), false)
			policy := initialPolicy()
			policy.Assignments = nil
			initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
			require.NoError(t, err)
			require.NoError(t, initializer.Initialize(t.Context(), policy))
			guard := &bootstrapSettingsGuard{}
			if scenario != "missing guard" {
				store.settings = guard
			}
			if scenario == "denied" {
				guard.err = errors.New("Kubernetes bootstrap check denied")
			}
			if scenario == "workload" {
				guard.principals = []api.Principal{testPrincipal(api.PrincipalTypeWorkload, "kubernetes:cluster-a", "service")}
			}
			_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{
				Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)},
			})
			require.Error(t, err)
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			got, err := snapshot.Policy()
			require.NoError(t, err)
			require.Zero(t, got.Generation)
			require.Empty(t, got.Assignments)
			require.Equal(t, api.UserAuthorizationModeDisabled, *got.Settings.Properties.Mode)
		})
	}
}

func TestPolicyBootstrapAdditiveChangesAndPromotion(t *testing.T) {
	store := newTestPolicyStore(t, inmemory.NewClient(), false)
	policy := initialPolicy()
	policy.Assignments = policy.Assignments[:1]
	policy.Assignments[0].Properties.ManagedBy.Kind = to.Ptr(api.AssignmentManagementKindBootstrap)
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	_, err = store.Mutate(t.Context(), policyActor(), readerMutation())
	require.NoError(t, err)
	roleID := teamA + "/providers/Radius.Core/roleDefinitions/additive"
	_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read").Properties})
	require.NoError(t, err)
	// Promotion uses the existing ID and tuple, not a duplicate assignment.
	_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{Target: *policy.Assignments[0].ID, Assignment: policy.Assignments[0].Properties})
	require.NoError(t, err)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	got, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, 1, permanentAdministrators(got))
	require.Len(t, got.Assignments, 2)
	event, err := store.AccessChange(t.Context(), 3)
	require.NoError(t, err)
	require.True(t, *event.Properties.HighImpact)
	require.Equal(t, api.AssignmentManagementKindBootstrap, *event.Properties.PolicyChange.Before.RoleAssignment.ManagedBy.Kind)
	require.Equal(t, api.AssignmentManagementKindImperative, *event.Properties.PolicyChange.After.RoleAssignment.ManagedBy.Kind)
	require.Equal(t, *policy.Assignments[0].Properties.CreatedBy, *event.Properties.PolicyChange.After.RoleAssignment.CreatedBy)
	_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{Target: *policy.Assignments[0].ID, Delete: true})
	require.ErrorContains(t, err, "LastAdministratorProtected")
}

func TestPolicyBootstrapSettingsRecovery(t *testing.T) {
	db := inmemory.NewClient()
	store := newTestPolicyStore(t, db, false)
	policy := initialPolicy()
	policy.Assignments = nil
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	interrupted := errors.New("publication interrupted")
	store.db = &policyHookClient{Client: db, before: func(_ context.Context, object *database.Object) error {
		if saveStage(object) == "finalize" {
			return interrupted
		}
		return nil
	}}
	store.settings = &bootstrapSettingsGuard{principals: []api.Principal{policyActor().Identity.Principal}}
	_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{
		Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)},
	})
	require.ErrorIs(t, err, interrupted)
	recovered, err := newTestPolicyStore(t, db, false).LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	got, err := recovered.Policy()
	require.NoError(t, err)
	require.Equal(t, int64(1), got.Generation)
	require.Equal(t, api.UserAuthorizationModeDryRun, *got.Settings.Properties.Mode)
	require.Len(t, got.Assignments, 1, "recovery publishes settings and bootstrap administrator together")
	require.NoError(t, recovered.allows(policyActor().Identity, "Radius.Core/authorizationSettings/write", settingsID))
}
