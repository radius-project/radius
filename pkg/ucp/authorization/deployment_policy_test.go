// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestDeploymentPolicyChunkedHistoryRecovery(t *testing.T) {
	runDeploymentPolicyChunkedHistoryRecovery(t, func(*testing.T) database.Client { return inmemory.NewClient() })
}

func runDeploymentPolicyChunkedHistoryRecovery(t *testing.T, newClient func(*testing.T) database.Client) {
	t.Helper()
	for _, stage := range []string{"chunk", "event", "claim", "history", "receipt", "finalize"} {
		for _, afterWrite := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/after=%v", stage, afterWrite), func(t *testing.T) {
				t.Parallel()
				client := newClient(t)
				store := newTestPolicyStore(t, client, true)
				mutations := deploymentMutations()
				mutations[0].Assignment.Description = to.Ptr(strings.Repeat("x", 90000))
				mutations[1].Role.Description = to.Ptr(strings.Repeat("x", 90000))
				before, err := store.ApplyDeployment(t.Context(), "large-initial", 0, deploymentActor(), mutations)
				require.NoError(t, err)
				require.Equal(t, int64(2), before.Generation())
				mutations[0].Assignment.Description = to.Ptr(strings.Repeat("y", 90000))
				mutations[1].Role.Description = to.Ptr(strings.Repeat("y", 90000))
				crash := errors.New("interrupted chunked deployment")
				var fired atomic.Bool
				hook := func(_ context.Context, object *database.Object) error {
					matches := saveStage(object) == stage
					switch stage {
					case "chunk":
						matches = strings.HasSuffix(object.ID, "-event-1-chunk-0")
					case "event":
						matches = strings.HasSuffix(object.ID, "-event-1")
					case "receipt":
						matches = object.ID == deploymentReceiptID("large-update")
					}
					if matches && fired.CompareAndSwap(false, true) {
						return crash
					}
					return nil
				}
				wrapped := &policyHookClient{Client: client}
				wrapped.before = func(ctx context.Context, object *database.Object) error {
					raw, err := json.Marshal(object.Data)
					require.NoError(t, err)
					require.LessOrEqual(t, len(raw), maxPolicyBytes)
					if !afterWrite {
						return hook(ctx, object)
					}
					return nil
				}
				if afterWrite {
					wrapped.after = hook
				}
				store.db = wrapped
				_, err = store.ApplyDeployment(t.Context(), "large-update", before.Generation(), deploymentActor(), mutations)
				require.ErrorIs(t, err, crash)
				require.True(t, fired.Load())
				restarted := newTestPolicyStore(t, client, false)
				recovered, err := restarted.LoadPolicy(t.Context(), -1)
				require.NoError(t, err)
				if stage == "chunk" || stage == "event" || (stage == "claim" && !afterWrite) {
					require.Equal(t, before.Generation(), recovered.Generation())
				} else {
					require.Equal(t, before.Generation()+2, recovered.Generation())
				}
				retry, err := restarted.ApplyDeployment(t.Context(), "large-update", before.Generation(), deploymentActor(), mutations)
				require.NoError(t, err)
				require.Equal(t, int64(4), retry.Generation())
				for generation := int64(3); generation <= 4; generation++ {
					event, err := restarted.AccessChange(t.Context(), generation)
					require.NoError(t, err)
					change := event.Properties.PolicyChange
					var oldDescription, newDescription *string
					if change.Before.RoleAssignment != nil {
						oldDescription, newDescription = change.Before.RoleAssignment.Description, change.After.RoleAssignment.Description
					} else {
						oldDescription, newDescription = change.Before.RoleDefinition.Description, change.After.RoleDefinition.Description
					}
					require.Equal(t, strings.Repeat("x", 90000), *oldDescription)
					require.Equal(t, strings.Repeat("y", 90000), *newDescription)
					var manifest historyManifest
					_, err = restarted.read(t.Context(), *event.ID, &manifest)
					require.NoError(t, err)
					require.Len(t, manifest.Chunks, 3)
				}
				deleted, err := restarted.Mutate(t.Context(), policyActor(), PolicyMutation{Target: mutations[0].Target, Delete: true})
				require.NoError(t, err)
				retry, err = restarted.ApplyDeployment(t.Context(), "large-update", before.Generation(), deploymentActor(), mutations)
				require.NoError(t, err)
				require.Equal(t, deleted.Generation(), retry.Generation(), "receipt cannot resurrect a revoked grant")
			})
		}
	}
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
