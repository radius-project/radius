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
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	clocktesting "k8s.io/utils/clock/testing"
)

type initializationVerifierFunc func(context.Context, Policy) error

func (f initializationVerifierFunc) VerifyPolicyInitialization(ctx context.Context, policy Policy) error {
	return f(ctx, policy)
}

type settingsGuardFunc func(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) error

func (f settingsGuardFunc) ValidateSettingsChange(ctx context.Context, actor PolicyActor, before, after api.AuthorizationSettingsProperties) error {
	return f(ctx, actor, before, after)
}

func policyActor() PolicyActor {
	identity := testIdentity()
	identity.Groups = []api.Principal{testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "admins")}
	return PolicyActor{
		Identity: identity, RequestID: "request-1", CorrelationID: "correlation-1",
		Management: api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)},
	}
}

func initialPolicy() Policy {
	actor := policyActor()
	a := testAssignment("alpha", builtInID("radius-administrator"), "/", actor.Identity.Principal)
	b := testAssignment("beta", builtInID("radius-administrator"), "/", actor.Identity.Groups[0])
	for _, assignment := range []*api.RoleAssignmentResource{&a, &b} {
		assignment.Properties.CreatedBy = to.Ptr("trusted-startup")
		assignment.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
	}
	return Policy{
		Assignments: []api.RoleAssignmentResource{a, b},
		Settings: api.AuthorizationSettingsResource{
			ID:         to.Ptr(settingsID),
			Properties: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDisabled), Generation: to.Ptr(int64(0))},
		},
	}
}

func newTestPolicyStore(t *testing.T, db database.Client, initialize bool) *PolicyStore {
	t.Helper()
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	store, err := NewPolicyStore(db, catalog, clocktesting.NewFakeClock(time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)), nil)
	require.NoError(t, err)
	if initialize {
		initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
		require.NoError(t, err)
		require.NoError(t, initializer.Initialize(t.Context(), initialPolicy()))
	}
	return store
}

func assignmentMutation(name, role, scope, subject string) PolicyMutation {
	a := testAssignment(name, role, scope, testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", subject))
	return PolicyMutation{Target: *a.ID, Assignment: a.Properties}
}

func readerMutation() PolicyMutation {
	return assignmentMutation("reader", builtInID("reader"), teamA, "bob")
}

type policyHookClient struct {
	database.Client
	before func(context.Context, *database.Object) error
	after  func(context.Context, *database.Object) error
}

func (c *policyHookClient) Save(ctx context.Context, obj *database.Object, opts ...database.SaveOptions) error {
	if c.before != nil {
		if err := c.before(ctx, obj); err != nil {
			return err
		}
	}
	if err := c.Client.Save(ctx, obj, opts...); err != nil {
		return err
	}
	if c.after != nil {
		return c.after(ctx, obj)
	}
	return nil
}

func saveStage(obj *database.Object) string {
	if obj.ID == accessVersionID {
		raw, err := json.Marshal(obj.Data)
		if err != nil {
			return "invalid"
		}
		var version accessVersion
		if json.Unmarshal(raw, &version) != nil {
			return "invalid"
		}
		if version.Pending != nil {
			return "claim"
		}
		return "finalize"
	}
	if strings.HasPrefix(obj.ID, historyPrefix) {
		return "history"
	}
	for _, suffix := range []string{"-event", "-root", "-page-", "-object-"} {
		if strings.Contains(obj.ID, suffix) {
			return strings.Trim(suffix, "-")
		}
	}
	return "other"
}

func TestPolicyStoreInitialization(t *testing.T) {
	runPolicyStoreInitialization(t, inmemory.NewClient())
}

func runPolicyStoreInitialization(t *testing.T, db database.Client) {
	store := newTestPolicyStore(t, db, false)
	_, err := store.LoadPolicy(t.Context(), -1)
	require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	_, err = store.Mutate(t.Context(), policyActor(), readerMutation())
	require.ErrorIs(t, err, ErrAuthorizationUnavailable)
	_, err = NewPolicyInitializer(store, nil)
	require.Error(t, err)
	denied := errors.New("untrusted startup caller")
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return denied }))
	require.NoError(t, err)
	require.ErrorIs(t, initializer.Initialize(t.Context(), initialPolicy()), denied)
	_, err = store.LoadPolicy(t.Context(), -1)
	require.ErrorIs(t, err, ErrAuthorizationUnavailable)

	initializer, err = NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			results <- initializer.Initialize(t.Context(), initialPolicy())
		})
	}
	close(start)
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else {
			var conflict *database.ErrConcurrency
			require.ErrorAs(t, err, &conflict)
		}
	}
	require.Equal(t, 1, success)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, int64(0), snapshot.Generation())
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Len(t, policy.Assignments, 2)
}

// runPolicyStoreContract runs unchanged on memory and both real storage clients.
func runPolicyStoreContract(t *testing.T, db database.Client) {
	t.Helper()
	store := newTestPolicyStore(t, db, true)
	t.Run("durable_idempotent_history", func(t *testing.T) {
		cache, err := NewPolicyCache(store, nil)
		require.NoError(t, err)
		require.NoError(t, cache.Refresh(t.Context()))
		snapshot, err := store.MutateAndRefresh(t.Context(), cache, policyActor(), readerMutation())
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.Generation())
		require.Equal(t, int64(1), cache.value.Generation())
		snapshot, err = store.Mutate(t.Context(), policyActor(), readerMutation())
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.Generation())
		event, err := store.AccessChange(t.Context(), 1)
		require.NoError(t, err)
		require.Equal(t, "request-1", *event.Properties.RequestID)
		require.Equal(t, "alice", *event.Properties.Principal.Subject)
		require.Equal(t, api.PolicyChangeOperationCreate, *event.Properties.PolicyChange.Operation)
		_, err = store.AccessChange(t.Context(), 2)
		var missing *database.ErrNotFound
		require.ErrorAs(t, err, &missing)
		// Restart does not change policy or generation.
		restarted := newTestPolicyStore(t, db, false)
		snapshot, err = restarted.LoadPolicy(t.Context(), -1)
		require.NoError(t, err)
		require.Equal(t, int64(1), snapshot.Generation())
	})
	t.Run("concurrent_admin_removals", func(t *testing.T) {
		var claims atomic.Int32
		reached := make(chan struct{}, 2)
		release := make(chan struct{})
		wrapped := &policyHookClient{Client: db, before: func(ctx context.Context, obj *database.Object) error {
			if saveStage(obj) == "claim" && claims.Add(1) <= 2 {
				reached <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}}
		first, second := newTestPolicyStore(t, wrapped, false), newTestPolicyStore(t, wrapped, false)
		results := make(chan error, 2)
		for n, name := range []string{"alpha", "beta"} {
			go func() {
				chosen := first
				if n == 1 {
					chosen = second
				}
				_, err := chosen.Mutate(t.Context(), policyActor(), PolicyMutation{
					Target: "/providers/Radius.Core/roleAssignments/" + name, Delete: true,
				})
				results <- err
			}()
		}
		<-reached
		<-reached
		close(release)
		a, b := <-results, <-results
		if a != nil {
			a, b = b, a
		}
		require.NoError(t, a)
		require.ErrorContains(t, b, "LastAdministratorProtected")
		snapshot, err := store.LoadPolicy(t.Context(), -1)
		require.NoError(t, err)
		require.Equal(t, int64(2), snapshot.Generation())
		policy, err := snapshot.Policy()
		require.NoError(t, err)
		require.Equal(t, 1, permanentAdministrators(policy))
		history, err := store.AccessChange(t.Context(), 2)
		require.NoError(t, err)
		require.Equal(t, int64(2), *history.Properties.Generation)
		t.Logf("generation=%d permanentAdministrators=%d committed history=1..2", snapshot.Generation(), permanentAdministrators(policy))
	})
}

func TestPolicyStoreContract(t *testing.T) {
	runPolicyStoreContract(t, inmemory.NewClient())
}

func runPolicyRoleRaces(t *testing.T, newClient func(*testing.T) database.Client) {
	for _, operation := range []string{"delete", "restrict scopes"} {
		t.Run(operation, func(t *testing.T) {
			db := newClient(t)
			store := newTestPolicyStore(t, db, true)
			roleID := teamA + "/providers/Radius.Core/roleDefinitions/operator"
			_, err := store.Mutate(t.Context(), policyActor(), PolicyMutation{
				Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read").Properties,
			})
			require.NoError(t, err)
			var claims atomic.Int32
			reached, release := make(chan struct{}, 2), make(chan struct{})
			wrapped := &policyHookClient{Client: db, before: func(ctx context.Context, obj *database.Object) error {
				if saveStage(obj) == "claim" && claims.Add(1) <= 2 {
					reached <- struct{}{}
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			}}
			mutation := PolicyMutation{Target: roleID, Delete: true}
			if operation != "delete" {
				mutation.Delete = false
				mutation.Role = customRole(roleID, "Radius.Core/applications/read").Properties
				mutation.Role.AssignableScopes = to.SliceOfPtrs(teamB)
			}
			first, second := newTestPolicyStore(t, wrapped, false), newTestPolicyStore(t, wrapped, false)
			results := make(chan error, 2)
			go func() {
				_, err := first.Mutate(t.Context(), policyActor(), mutation)
				results <- err
			}()
			go func() {
				_, err := second.Mutate(t.Context(), policyActor(), assignmentMutation("operator", roleID, teamA, "bob"))
				results <- err
			}()
			<-reached
			<-reached
			close(release)
			a, b := <-results, <-results
			require.NotEqual(t, a == nil, b == nil, "exactly one conflicting mutation must succeed: %v; %v", a, b)
			if a == nil {
				a = b
			}
			if operation == "delete" {
				require.True(t, strings.Contains(a.Error(), "RoleDefinitionInUse") || strings.Contains(a.Error(), "unknown role definition"), a.Error())
			} else {
				require.ErrorContains(t, a, "ScopeNotAssignable")
			}
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			require.Equal(t, int64(2), snapshot.Generation())
		})
	}
}

func TestPolicyStoreRoleRaces(t *testing.T) {
	runPolicyRoleRaces(t, func(t *testing.T) database.Client { return inmemory.NewClient() })
}

func TestPolicyStoreCrashRecovery(t *testing.T) {
	runPolicyStoreCrashRecovery(t, func(t *testing.T) database.Client { return inmemory.NewClient() })
}

func runPolicyStoreCrashRecovery(t *testing.T, newClient func(*testing.T) database.Client) {
	for _, stage := range []string{"object", "page", "root", "event", "claim", "history", "finalize"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/after=%v", stage, after), func(t *testing.T) {
				db := newClient(t)
				newTestPolicyStore(t, db, true)
				crash := errors.New("injected interruption")
				fired := false
				hook := func(_ context.Context, obj *database.Object) error {
					if !fired && saveStage(obj) == stage {
						fired = true
						return crash
					}
					return nil
				}
				wrapped := &policyHookClient{Client: db}
				if after {
					wrapped.after = hook
				} else {
					wrapped.before = hook
				}
				store := newTestPolicyStore(t, wrapped, false)
				_, err := store.Mutate(t.Context(), policyActor(), readerMutation())
				require.True(t, fired)
				require.ErrorIs(t, err, crash)
				restarted := newTestPolicyStore(t, db, false)
				snapshot, err := restarted.LoadPolicy(t.Context(), -1)
				require.NoError(t, err)
				claimed := stage == "history" || stage == "finalize" || stage == "claim" && after
				if claimed {
					require.Equal(t, int64(1), snapshot.Generation())
				} else {
					require.Equal(t, int64(0), snapshot.Generation())
				}
				snapshot, err = restarted.Mutate(t.Context(), policyActor(), readerMutation())
				require.NoError(t, err)
				require.Equal(t, int64(1), snapshot.Generation())
				event, err := restarted.AccessChange(t.Context(), 1)
				require.NoError(t, err)
				require.Equal(t, "request-1", *event.Properties.RequestID)
				require.Equal(t, time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), *event.Properties.Time)
				items, err := db.Query(t.Context(), database.Query{RootScope: "/", ResourceType: "Radius.Core/accessChanges", ScopeRecursive: true})
				require.NoError(t, err)
				require.Len(t, items.Items, 1)
			})
		}
	}
}

func TestPolicyStoreDelayedRecoveryCannotResurrectAssignment(t *testing.T) {
	runPolicyStoreDelayedRecovery(t, inmemory.NewClient())
}

func runPolicyStoreDelayedRecovery(t *testing.T, db database.Client) {
	newTestPolicyStore(t, db, true)
	crash := errors.New("crash after claiming intent")
	claimCrash := &policyHookClient{Client: db, after: func(_ context.Context, obj *database.Object) error {
		if saveStage(obj) == "claim" {
			return crash
		}
		return nil
	}}
	_, err := newTestPolicyStore(t, claimCrash, false).Mutate(t.Context(), policyActor(), readerMutation())
	require.ErrorIs(t, err, crash)
	reached, release := make(chan struct{}), make(chan struct{})
	var paused atomic.Bool
	delayed := &policyHookClient{Client: db, before: func(ctx context.Context, obj *database.Object) error {
		if saveStage(obj) == "history" && paused.CompareAndSwap(false, true) {
			close(reached)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}}
	result := make(chan error, 1)
	delayedStore := newTestPolicyStore(t, delayed, false)
	go func() {
		_, err := delayedStore.LoadPolicy(t.Context(), -1)
		result <- err
	}()
	<-reached
	store := newTestPolicyStore(t, db, false)
	store.clock = clocktesting.NewFakeClock(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	recovered, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, int64(1), recovered.Generation())
	deleted, err := store.Mutate(t.Context(), policyActor(), PolicyMutation{Target: readerMutation().Target, Delete: true})
	require.NoError(t, err)
	require.Equal(t, int64(2), deleted.Generation())
	close(release)
	require.NoError(t, <-result)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Len(t, policy.Assignments, 2)
	require.Equal(t, int64(2), snapshot.Generation())
	for generation := int64(1); generation <= 2; generation++ {
		event, err := store.AccessChange(t.Context(), generation)
		require.NoError(t, err)
		require.Equal(t, generation, *event.Properties.Generation)
		if generation == 1 {
			require.Equal(t, time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC), *event.Properties.Time)
		}
	}
	t.Log("paused generation-1 recoverer resumed after generation-2 delete: no resurrection, one event per generation")
}

func TestPolicyStoreConflictsCancellationAndLimits(t *testing.T) {
	db := inmemory.NewClient()
	newTestPolicyStore(t, db, true)
	var attempts atomic.Int32
	wrapped := &policyHookClient{Client: db, before: func(_ context.Context, obj *database.Object) error {
		if saveStage(obj) == "claim" {
			attempts.Add(1)
			return &database.ErrConcurrency{}
		}
		return nil
	}}
	store := newTestPolicyStore(t, wrapped, false)
	_, err := store.Mutate(t.Context(), policyActor(), readerMutation())
	require.ErrorIs(t, err, ErrPolicyConflict)
	require.Equal(t, int32(6), attempts.Load())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = store.Mutate(ctx, policyActor(), readerMutation())
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int32(6), attempts.Load())
	mutation := readerMutation()
	mutation.Assignment.Description = to.Ptr(strings.Repeat("x", maxPolicyBytes))
	_, err = newTestPolicyStore(t, db, false).Mutate(t.Context(), policyActor(), mutation)
	require.ErrorContains(t, err, "PolicyCapacityExceeded")
}

func TestPolicyStoreSettings(t *testing.T) {
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	mutation := PolicyMutation{Target: settingsID, Settings: &api.AuthorizationSettingsProperties{
		Mode: to.Ptr(api.UserAuthorizationModeDryRun), Generation: to.Ptr(int64(999)),
	}}
	denied := errors.New("bootstrap SubjectAccessReview denied")
	store.settings = settingsGuardFunc(func(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) error {
		return denied
	})
	_, err := store.Mutate(t.Context(), policyActor(), mutation)
	require.ErrorIs(t, err, denied)
	store.settings = settingsGuardFunc(func(_ context.Context, actor PolicyActor, before, after api.AuthorizationSettingsProperties) error {
		require.Equal(t, "request-1", actor.RequestID)
		require.Equal(t, api.UserAuthorizationModeDisabled, *before.Mode)
		require.Equal(t, api.UserAuthorizationModeDryRun, *after.Mode)
		return nil
	})
	snapshot, err := store.Mutate(t.Context(), policyActor(), mutation)
	require.NoError(t, err)
	require.Equal(t, int64(1), snapshot.Generation())
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, int64(1), *policy.Settings.Properties.Generation)
	event, err := store.AccessChange(t.Context(), 1)
	require.NoError(t, err)
	require.True(t, *event.Properties.HighImpact)
	require.Equal(t, api.UserAuthorizationModeDisabled, *event.Properties.PolicyChange.Before.AuthorizationSettings.Mode)
	require.Equal(t, api.UserAuthorizationModeDryRun, *event.Properties.PolicyChange.After.AuthorizationSettings.Mode)
	require.Equal(t, int64(1), *event.Properties.PolicyChange.After.AuthorizationSettings.Generation)
	store.settings = settingsGuardFunc(func(context.Context, PolicyActor, api.AuthorizationSettingsProperties, api.AuthorizationSettingsProperties) error {
		return nil
	})
	snapshot, err = store.Mutate(t.Context(), policyActor(), mutation)
	require.NoError(t, err)
	require.Equal(t, int64(1), snapshot.Generation(), "server-owned generation cannot turn an identical mode PUT into a change")
}

func fullCapacityPolicy() Policy {
	policy := initialPolicy()
	for n := range 1000 {
		id := fmt.Sprintf("%s/providers/Radius.Core/roleDefinitions/custom-%d", teamA, n)
		policy.CustomRoles = append(policy.CustomRoles, customRole(id, "Radius.Core/applications/read"))
	}
	for n := range 9998 {
		a := testAssignment(fmt.Sprintf("reader-%d", n), builtInID("reader"), teamA,
			testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", fmt.Sprintf("user-%d", n)))
		a.Properties.CreatedBy = to.Ptr("trusted-startup")
		a.Properties.ManagedBy = &api.AssignmentManagement{Kind: to.Ptr(api.AssignmentManagementKindImperative)}
		policy.Assignments = append(policy.Assignments, a)
	}
	return policy
}

func TestPolicyStoreManifestCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("full 10000 assignment and 1000 role capacity exercise")
	}
	db := inmemory.NewClient()
	store := newTestPolicyStore(t, db, false)
	policy := fullCapacityPolicy()
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	var version accessVersion
	obj, err := store.read(t.Context(), accessVersionID, &version)
	require.NoError(t, err)
	raw, err := json.Marshal(obj.Data)
	require.NoError(t, err)
	require.Less(t, len(raw), 1024)
	var root manifestRoot
	_, err = store.read(t.Context(), version.Root, &root)
	require.NoError(t, err)
	require.Len(t, root.Pages, 86)
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	copy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Len(t, copy.Assignments, 10000)
	require.Len(t, copy.CustomRoles, 1000)
	_, err = store.Mutate(t.Context(), policyActor(), readerMutation())
	require.ErrorContains(t, err, "PolicyCapacityExceeded")
	roleID := teamA + "/providers/Radius.Core/roleDefinitions/over-capacity"
	_, err = store.Mutate(t.Context(), policyActor(), PolicyMutation{
		Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read").Properties,
	})
	require.ErrorContains(t, err, "PolicyCapacityExceeded")
	t.Logf("assignments=%d roles=%d pages=%d anchorBytes=%d", len(copy.Assignments), len(copy.CustomRoles), len(root.Pages), len(raw))
}

func TestPolicyStorePayloadByteBound(t *testing.T) {
	for _, over := range []bool{false, true} {
		t.Run(fmt.Sprintf("over=%v", over), func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), false)
			policy := initialPolicy()
			assignment := policy.Assignments[0]
			assignment.Properties.Description = to.Ptr("")
			raw, err := json.Marshal(assignment)
			require.NoError(t, err)
			key, err := json.Marshal(strings.ToLower(*assignment.ID))
			require.NoError(t, err)
			size := maxPolicyBytes - len(`{"entries":{}}`) - len(key) - 1 - len(raw)
			if over {
				size++
			}
			assignment.Properties.Description = to.Ptr(strings.Repeat("x", size))
			policy.Assignments[0] = assignment
			initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
			require.NoError(t, err)
			err = initializer.Initialize(t.Context(), policy)
			if over {
				require.ErrorContains(t, err, "PolicyCapacityExceeded")
				return
			}
			require.NoError(t, err)
			snapshot, err := store.LoadPolicy(t.Context(), -1)
			require.NoError(t, err)
			actual, err := snapshot.Policy()
			require.NoError(t, err)
			require.ElementsMatch(t, policy.Assignments, actual.Assignments)
			var version accessVersion
			_, err = store.read(t.Context(), accessVersionID, &version)
			require.NoError(t, err)
			var root manifestRoot
			_, err = store.read(t.Context(), version.Root, &root)
			require.NoError(t, err)
			require.Len(t, root.Pages, 3, "isolate the full-size payload from entries sorting before and after it")
			largest := 0
			for _, id := range root.Pages {
				var page manifestPage
				_, err = store.read(t.Context(), id, &page)
				require.NoError(t, err)
				require.NotEmpty(t, page.Entries)
				var payload json.RawMessage
				_, err = store.read(t.Context(), page.Object, &payload)
				require.NoError(t, err)
				largest = max(largest, len(payload))
			}
			require.Equal(t, maxPolicyBytes, largest)
		})
	}
}

func TestPolicyStorePayloadValidation(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		change func(*manifestPage, *policyPayload)
		error  string
	}{
		{"invalid payload reference", func(page *manifestPage, _ *policyPayload) {
			page.Object = "/providers/Radius.Core/roleAssignments/alpha"
		}, "invalid private policy payload"},
		{"missing entry", func(page *manifestPage, payload *policyPayload) {
			delete(payload.Entries, page.Entries[0].ID)
		}, "entry count mismatch"},
		{"extra entry", func(_ *manifestPage, payload *policyPayload) {
			payload.Entries["unexpected"] = json.RawMessage(`{}`)
		}, "entry count mismatch"},
		{"wrong entry key", func(page *manifestPage, payload *policyPayload) {
			id := page.Entries[0].ID
			payload.Entries["unexpected"] = payload.Entries[id]
			delete(payload.Entries, id)
		}, "reference missing from payload"},
		{"mismatched assignment ID", func(page *manifestPage, payload *policyPayload) {
			for _, ref := range page.Entries {
				if ref.Kind == "assignment" {
					payload.Entries[ref.ID] = json.RawMessage(`{"id":"wrong"}`)
					return
				}
			}
		}, "assignment manifest ID mismatch"},
		{"duplicate reference", func(page *manifestPage, _ *policyPayload) {
			page.Entries[1] = page.Entries[0]
		}, "duplicate or invalid policy reference"},
		{"unknown entry kind", func(page *manifestPage, _ *policyPayload) {
			page.Entries[0].Kind = "unknown"
		}, "unknown policy entry kind"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), true)
			var version accessVersion
			_, err := store.read(t.Context(), accessVersionID, &version)
			require.NoError(t, err)
			var root manifestRoot
			_, err = store.read(t.Context(), version.Root, &root)
			require.NoError(t, err)
			var page manifestPage
			_, err = store.read(t.Context(), root.Pages[0], &page)
			require.NoError(t, err)
			var payload policyPayload
			_, err = store.read(t.Context(), page.Object, &payload)
			require.NoError(t, err)
			payloadID := page.Object
			scenario.change(&page, &payload)
			require.NoError(t, store.save(t.Context(), root.Pages[0], page))
			require.NoError(t, store.save(t.Context(), payloadID, payload))
			_, err = store.LoadPolicy(t.Context(), -1)
			require.ErrorContains(t, err, scenario.error)
		})
	}
}
