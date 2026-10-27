// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	clocktesting "k8s.io/utils/clock/testing"
)

type policySourceFunc func(context.Context, int64) (*PolicySnapshot, error)

func (f policySourceFunc) LoadPolicy(ctx context.Context, generation int64) (*PolicySnapshot, error) {
	return f(ctx, generation)
}

func testPolicySnapshot(t *testing.T, generation int64) *PolicySnapshot {
	t.Helper()
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	snapshot, err := NewPolicySnapshot(catalog, Policy{
		Generation: generation,
		Assignments: []api.RoleAssignmentResource{
			testAssignment("admin", builtInID("radius-administrator"), "/", testIdentity().Principal),
		},
		Settings: api.AuthorizationSettingsResource{
			Properties: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDisabled)},
		},
	})
	require.NoError(t, err)
	return snapshot
}

func TestPolicyCacheFreshness(t *testing.T) {
	for _, age := range []time.Duration{0, PolicyMaxAge - time.Nanosecond, PolicyMaxAge, PolicyMaxAge + time.Nanosecond} {
		t.Run(age.String(), func(t *testing.T) {
			clk := clocktesting.NewFakeClock(time.Now())
			snapshot := testPolicySnapshot(t, 1)
			fail := false
			cache, err := NewPolicyCache(policySourceFunc(func(context.Context, int64) (*PolicySnapshot, error) {
				if fail {
					return nil, errors.New("database unavailable")
				}
				return snapshot, nil
			}), clk)
			require.NoError(t, err)
			require.ErrorIs(t, cache.Ready(), ErrAuthorizationUnavailable)
			require.NoError(t, cache.Refresh(t.Context()))
			fail = true
			clk.Step(age)
			require.ErrorContains(t, cache.Refresh(t.Context()), "database unavailable")
			decisions, err := cache.Evaluate(testIdentity(), []Check{{"Radius.Core/applications/write", appA}})
			if age >= PolicyMaxAge {
				require.ErrorIs(t, err, ErrAuthorizationUnavailable)
				require.ErrorIs(t, cache.Ready(), ErrAuthorizationUnavailable)
				require.Nil(t, decisions)
			} else {
				require.NoError(t, err)
				require.NoError(t, cache.Ready())
				require.True(t, decisions[0].Allowed)
			}
			fail = false
			require.NoError(t, cache.Refresh(t.Context()))
			require.NoError(t, cache.Ready())
		})
	}
}

func TestPolicyCacheRefreshValidation(t *testing.T) {
	for _, scenario := range []string{"unchanged", "regressed", "slow", "cancelled", "new generation"} {
		t.Run(scenario, func(t *testing.T) {
			clk := clocktesting.NewFakeClock(time.Now())
			first, second := testPolicySnapshot(t, 2), testPolicySnapshot(t, 3)
			calls := 0
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			cache, err := NewPolicyCache(policySourceFunc(func(_ context.Context, known int64) (*PolicySnapshot, error) {
				calls++
				if calls == 1 {
					require.Equal(t, int64(-1), known)
					return first, nil
				}
				require.Equal(t, int64(2), known)
				switch scenario {
				case "unchanged":
					return nil, nil
				case "regressed":
					return testPolicySnapshot(t, 1), nil
				case "slow":
					clk.Step(PolicyMaxAge)
				case "cancelled":
					cancel()
				}
				return second, nil
			}), clk)
			require.NoError(t, err)
			require.NoError(t, cache.Refresh(ctx))
			clk.Step(PolicyRefreshInterval)
			err = cache.Refresh(ctx)
			if scenario == "unchanged" || scenario == "new generation" {
				require.NoError(t, err)
				require.NoError(t, cache.Ready())
			} else {
				require.Error(t, err)
				require.Equal(t, int64(2), cache.value.Generation())
			}
		})
	}
}

func TestPolicySnapshotImmutable(t *testing.T) {
	snapshot := testPolicySnapshot(t, 1)
	policy, err := snapshot.Policy()
	require.NoError(t, err)
	policy.Assignments[0].Properties.Principal.Subject = to.Ptr("mallory")
	policy.Assignments = nil
	decisions, err := snapshot.Evaluate(testIdentity(), []Check{{"Radius.Core/applications/write", appA}})
	require.NoError(t, err)
	require.True(t, decisions[0].Allowed)
	copy, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, "alice", *copy.Assignments[0].Properties.Principal.Subject)
}

func TestPolicyCacheRunAndShutdown(t *testing.T) {
	clk := clocktesting.NewFakeClock(time.Now())
	snapshot := testPolicySnapshot(t, 1)
	called := make(chan struct{}, 3)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var mu sync.Mutex
	failed := false
	cache, err := NewPolicyCache(policySourceFunc(func(context.Context, int64) (*PolicySnapshot, error) {
		mu.Lock()
		defer mu.Unlock()
		called <- struct{}{}
		if failed {
			return nil, errors.New("reload failed")
		}
		return snapshot, nil
	}), clk)
	require.NoError(t, err)
	reported := make(chan error, 1)
	done := make(chan error, 1)
	go func() { done <- cache.Run(ctx, func(err error) { reported <- err }) }()
	<-called
	require.Eventually(t, func() bool { return cache.Ready() == nil }, time.Second, time.Millisecond)
	mu.Lock()
	failed = true
	mu.Unlock()
	clk.Step(PolicyRefreshInterval)
	<-called
	require.ErrorContains(t, <-reported, "reload failed")
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

func TestPolicyCacheCancelledWaiter(t *testing.T) {
	cache, err := NewPolicyCache(policySourceFunc(func(context.Context, int64) (*PolicySnapshot, error) {
		t.Fatal("cancelled waiter must not load")
		return nil, nil
	}), nil)
	require.NoError(t, err)
	cache.gate <- struct{}{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, cache.Refresh(ctx), context.Canceled)
	<-cache.gate
}

func TestPolicySnapshotIndexEquivalence(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	policy := initialPolicy()
	policy.Assignments = append(policy.Assignments,
		testAssignment("group-reader", builtInID("reader"), teamA, policyActor().Identity.Groups[0]),
		testAssignment("environment", builtInID("environment-deployer"), envA, testIdentity().Principal))
	snapshot, err := NewPolicySnapshot(catalog, policy)
	require.NoError(t, err)
	for _, scenario := range []struct {
		name     string
		identity Identity
		checks   []Check
	}{
		{"user inheritance", testIdentity(), []Check{{"Radius.Core/applications/write", appA}}},
		{"group inheritance", policyActor().Identity, []Check{{"Radius.Core/applications/read", appA}}},
		{"sibling scope", policyActor().Identity, []Check{{"Radius.Core/applications/read", teamB}}},
		{"resource scope", testIdentity(), []Check{{"Radius.Core/environments/use/action", envA}}},
		{"multiple targets", testIdentity(), []Check{{"Radius.Core/applications/read", appA}, {"Radius.Core/environments/use/action", envA}}},
		{"absent principal", Identity{Principal: testPrincipal(api.PrincipalTypeUser, "kubernetes:cluster-a", "mallory")}, []Check{{"Radius.Core/applications/read", appA}}},
		{"foreign group issuer", Identity{Principal: testIdentity().Principal, Groups: []api.Principal{testPrincipal(api.PrincipalTypeGroup, "foreign", "admins")}}, []Check{{"Radius.Core/applications/read", appA}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			expected, expectedErr := snapshot.evaluator.Evaluate(scenario.identity, scenario.checks)
			actual, actualErr := snapshot.Evaluate(scenario.identity, scenario.checks)
			if expectedErr != nil {
				require.Error(t, actualErr)
			} else {
				require.NoError(t, actualErr)
				require.Equal(t, expected, actual)
			}
		})
	}
}

type rateLimitedPolicyClient struct {
	database.Client
	clock   *clocktesting.FakeClock
	limiter *rate.Limiter
	gets    int
}

func (c *rateLimitedPolicyClient) Get(ctx context.Context, id string, opts ...database.GetOptions) (*database.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	now := c.clock.Now()
	reservation := c.limiter.ReserveN(now, 1)
	c.clock.Step(reservation.DelayFrom(now))
	c.gets++
	return c.Client.Get(ctx, id, opts...)
}

func TestPolicyCacheRateLimitedCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("full 10000 assignment and 1000 role rate-limited refresh exercise")
	}
	db := inmemory.NewClient()
	writer := newTestPolicyStore(t, db, false)
	initializer, err := NewPolicyInitializer(writer, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), fullCapacityPolicy()))
	clk := clocktesting.NewFakeClock(time.Now())
	limited := &rateLimitedPolicyClient{Client: db, clock: clk, limiter: rate.NewLimiter(200, 200)}
	reader := newTestPolicyStore(t, limited, false)
	cache, err := NewPolicyCache(reader, clk)
	require.NoError(t, err)
	for _, generation := range []int64{0, 1} {
		if generation == 1 {
			roleID := teamA + "/providers/Radius.Core/roleDefinitions/custom-0"
			_, err := writer.Mutate(t.Context(), policyActor(), PolicyMutation{
				Target: roleID, Role: customRole(roleID, "Radius.Core/applications/read", "Radius.Core/applications/write").Properties,
			})
			require.NoError(t, err)
		}
		// Exhaust the burst so this checks sustained QPS, not just free tokens.
		require.True(t, limited.limiter.AllowN(clk.Now(), limited.limiter.Burst()))
		limited.gets = 0
		started := clk.Now()
		wallStarted := time.Now()
		require.NoError(t, cache.Refresh(t.Context()))
		elapsed := clk.Since(started) + time.Since(wallStarted)
		require.NoError(t, cache.Ready())
		require.Equal(t, generation, cache.value.Generation())
		require.Equal(t, 174, limited.gets, "one anchor, one root, 86 index pages and 86 payload chunks")
		require.Less(t, elapsed, PolicyMaxAge, "include encoding/validation CPU time as well as throttling")
		policy, err := cache.value.Policy()
		require.NoError(t, err)
		require.Len(t, policy.Assignments, 10000)
		require.Len(t, policy.CustomRoles, 1000)
		decisions, err := cache.Evaluate(testIdentity(), []Check{{"Radius.Core/applications/write", appA}})
		require.NoError(t, err)
		require.True(t, decisions[0].Allowed)
		t.Logf("generation=%d gets=%d rateLimitedDuration=%s ready=true", generation, limited.gets, clk.Since(started))
		clk.Step(PolicyRefreshInterval)
	}
}
