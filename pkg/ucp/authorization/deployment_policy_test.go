// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func deploymentActor() PolicyActor {
	actor := policyActor()
	actor.Management.Kind = to.Ptr(api.AssignmentManagementKindDeployment)
	return actor
}

func deploymentMutations() []PolicyMutation {
	role := teamA + "/providers/Radius.Core/roleDefinitions/deployed-reader"
	return []PolicyMutation{
		assignmentMutation("deployed-reader", role, teamA, "bob"),
		{Target: role, Role: &api.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/applications/read")}},
	}
}

func TestDeploymentPolicyAtomicityAndReceipt(t *testing.T) {
	t.Parallel()
	runDeploymentPolicyContract(t, inmemory.NewClient())
}

func runDeploymentPolicyContract(t *testing.T, client database.Client) {
	t.Helper()
	store := newTestPolicyStore(t, client, true)
	before, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	mutations := deploymentMutations()
	require.NoError(t, store.ValidateDeployment(t.Context(), before, deploymentActor(), mutations))
	after, err := store.ApplyDeployment(t.Context(), "execution-1", before.Generation(), deploymentActor(), mutations)
	require.NoError(t, err)
	require.Equal(t, before.Generation()+2, after.Generation())
	policy, err := after.Policy()
	require.NoError(t, err)
	require.Len(t, policy.CustomRoles, 1)
	for _, assignment := range policy.Assignments {
		if strings.EqualFold(*assignment.ID, mutations[0].Target) {
			require.Equal(t, api.AssignmentManagementKindDeployment, *assignment.Properties.ManagedBy.Kind)
		}
	}
	for generation := before.Generation() + 1; generation <= after.Generation(); generation++ {
		event, err := store.AccessChange(t.Context(), generation)
		require.NoError(t, err)
		require.Equal(t, "request-1", *event.Properties.RequestID)
		require.NotNil(t, event.Properties.PolicyChange)
	}
	retry, err := store.ApplyDeployment(t.Context(), "execution-1", before.Generation(), deploymentActor(), mutations)
	require.NoError(t, err)
	require.Equal(t, after.Generation(), retry.Generation())
	// A receipt is not a permanent approval to replay or restore old policy.
	changed, err := store.Mutate(t.Context(), policyActor(), PolicyMutation{
		Target: mutations[1].Target, Role: &api.RoleDefinitionProperties{Permissions: to.SliceOfPtrs("Radius.Core/applications/read"), Description: to.Ptr("later administrative change")},
	})
	require.NoError(t, err)
	retry, err = store.ApplyDeployment(t.Context(), "execution-1", before.Generation(), deploymentActor(), mutations)
	require.NoError(t, err)
	require.Equal(t, changed.Generation(), retry.Generation())
	policy, err = retry.Policy()
	require.NoError(t, err)
	require.Equal(t, "later administrative change", *policy.CustomRoles[0].Properties.Description)
	mutations[1].Role.Description = to.Ptr("forged retry")
	_, err = store.ApplyDeployment(t.Context(), "execution-1", before.Generation(), deploymentActor(), mutations)
	require.ErrorContains(t, err, "differs from its committed receipt")
}

func TestDeploymentPolicyUsesOriginalAuthority(t *testing.T) {
	t.Parallel()
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	role := teamA + "/providers/Radius.Core/roleDefinitions/delegated-admin"
	_, err := store.Mutate(t.Context(), policyActor(), PolicyMutation{Target: role, Role: &api.RoleDefinitionProperties{
		Permissions: to.SliceOfPtrs("Radius.Core/roleDefinitions/write", "Radius.Core/roleAssignments/write", "Radius.Core/applications/read"),
	}})
	require.NoError(t, err)
	_, err = store.Mutate(t.Context(), policyActor(), assignmentMutation("delegated-admin", role, teamA, "delegated"))
	require.NoError(t, err)
	before, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	actor := deploymentActor()
	actor.Identity.Principal.Subject = to.Ptr("delegated")
	actor.Identity.Groups = nil
	grant := assignmentMutation("original-authority", role, teamA, "bob")
	after, err := store.ApplyDeployment(t.Context(), "original-authority", before.Generation(), actor, []PolicyMutation{
		{Target: role, Role: &api.RoleDefinitionProperties{
			Permissions: to.SliceOfPtrs("Radius.Core/roleDefinitions/write", "Radius.Core/applications/read"),
		}},
		grant,
	})
	require.NoError(t, err)
	require.NoError(t, before.allows(actor.Identity, "Radius.Core/roleAssignments/write", grant.Target))
	require.ErrorContains(t, after.allows(actor.Identity, "Radius.Core/roleAssignments/write", grant.Target), "AuthorizationFailed")
}

func TestDeploymentPolicyRejectsEntireInvalidPlan(t *testing.T) {
	t.Parallel()
	for _, scenario := range []struct {
		name string
		edit func(*PolicyActor, *[]PolicyMutation)
		want string
	}{
		{"unknown permission", func(_ *PolicyActor, m *[]PolicyMutation) {
			(*m)[1].Role.Permissions = to.SliceOfPtrs("Radius.Core/unknown/write")
		}, "UnknownPermission"},
		{"missing role", func(_ *PolicyActor, m *[]PolicyMutation) {
			(*m)[0].Assignment.RoleDefinitionID = to.Ptr(teamA + "/providers/Radius.Core/roleDefinitions/missing")
		}, "unknown role"},
		{"duplicate target", func(_ *PolicyActor, m *[]PolicyMutation) { *m = append(*m, (*m)[1]) }, "distinct"},
		{"settings", func(_ *PolicyActor, m *[]PolicyMutation) {
			*m = append(*m, PolicyMutation{Target: settingsID, Settings: &api.AuthorizationSettingsProperties{}})
		}, "incremental"},
		{"deletion", func(_ *PolicyActor, m *[]PolicyMutation) { (*m)[1].Delete = true }, "incremental"},
		{"built-in", func(_ *PolicyActor, m *[]PolicyMutation) { (*m)[1].Target = builtInID("reader") }, "BuiltInRoleImmutable"},
		{"unauthorized", func(a *PolicyActor, _ *[]PolicyMutation) {
			a.Identity.Principal.Subject = to.Ptr("outsider")
			a.Identity.Groups = nil
		}, "AuthorizationFailed"},
		{"unverified provenance", func(a *PolicyActor, _ *[]PolicyMutation) {
			a.Management.Kind = to.Ptr(api.AssignmentManagementKindImperative)
		}, "verified deployment provenance"},
		{"assignment outside role scope", func(_ *PolicyActor, m *[]PolicyMutation) {
			(*m)[0] = assignmentMutation("outside", (*m)[1].Target, teamB, "bob")
		}, "ScopeNotAssignable"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			store := newTestPolicyStore(t, inmemory.NewClient(), true)
			before, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			actor, mutations := deploymentActor(), deploymentMutations()
			scenario.edit(&actor, &mutations)
			_, err = store.ApplyDeployment(t.Context(), "invalid-plan", before.Generation(), actor, mutations)
			require.ErrorContains(t, err, scenario.want)
			after, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			require.Equal(t, before.Generation(), after.Generation())
			policy, err := after.Policy()
			require.NoError(t, err)
			require.Empty(t, policy.CustomRoles)
			_, err = store.AccessChange(t.Context(), before.Generation()+1)
			require.Error(t, err)
		})
	}
}

func TestDeploymentPolicyGenerationBindingAndConcurrency(t *testing.T) {
	t.Parallel()
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	before, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			_, err := store.ApplyDeployment(t.Context(), "same-execution", before.Generation(), deploymentActor(), deploymentMutations())
			results <- err
		})
	}
	close(start)
	workers.Wait()
	for range 2 {
		require.NoError(t, <-results)
	}
	after, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, before.Generation()+2, after.Generation())
	_, err = store.ApplyDeployment(t.Context(), "different-execution", before.Generation(), deploymentActor(), deploymentMutations())
	require.ErrorIs(t, err, ErrPolicyConflict)
}

func TestDeploymentPolicyCrashRecovery(t *testing.T) {
	t.Parallel()
	runDeploymentPolicyCrashRecovery(t, func(*testing.T) database.Client { return inmemory.NewClient() })
}

func runDeploymentPolicyCrashRecovery(t *testing.T, newClient func(*testing.T) database.Client) {
	t.Helper()
	for _, stage := range []string{"claim", "event", "history", "receipt", "finalize"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			db := &policyHookClient{Client: newClient(t)}
			store := newTestPolicyStore(t, db, true)
			before, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			crashed := errors.New("simulated process loss")
			var once atomic.Bool
			db.after = func(_ context.Context, obj *database.Object) error {
				matches := saveStage(obj) == stage
				if stage == "receipt" {
					matches = strings.HasSuffix(obj.ID, "-receipt") || strings.Contains(obj.ID, "/policyDeployments/")
				}
				if matches && once.CompareAndSwap(false, true) {
					return crashed
				}
				return nil
			}
			_, err = store.ApplyDeployment(t.Context(), "recoverable-execution", before.Generation(), deploymentActor(), deploymentMutations())
			require.ErrorIs(t, err, crashed)
			require.True(t, once.Load())
			db.after = nil
			restarted := newTestPolicyStore(t, db, false)
			recovered, err := restarted.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			// A crash before claiming the intent publishes nothing. Every crash
			// after the claim recovers both resources and both history events.
			if stage == "event" || stage == "receipt" {
				require.Equal(t, before.Generation(), recovered.Generation())
				return
			}
			require.Equal(t, before.Generation()+2, recovered.Generation())
			retry, err := restarted.ApplyDeployment(t.Context(), "recoverable-execution", before.Generation(), deploymentActor(), deploymentMutations())
			require.NoError(t, err)
			require.Equal(t, recovered.Generation(), retry.Generation())
			for generation := before.Generation() + 1; generation <= recovered.Generation(); generation++ {
				_, err := restarted.AccessChange(t.Context(), generation)
				require.NoError(t, err)
			}
		})
	}
}
