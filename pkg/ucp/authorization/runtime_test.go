// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

type runtimeDatabase struct {
	database.Client
	mu  sync.RWMutex
	get func(context.Context, string) error
}

func (d *runtimeDatabase) setGet(get func(context.Context, string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.get = get
}

func (d *runtimeDatabase) Get(ctx context.Context, id string, opts ...database.GetOptions) (*database.Object, error) {
	d.mu.RLock()
	get := d.get
	d.mu.RUnlock()
	if get != nil {
		if err := get(ctx, id); err != nil {
			return nil, err
		}
	}
	return d.Client.Get(ctx, id, opts...)
}

// Called only inside a synctest bubble, where Wait joins each refresh without
// sleeps or polling. Every test owns and joins its runtime.
func startPolicyRuntime(t *testing.T, store *PolicyStore) (*PolicyRuntime, func()) {
	t.Helper()
	runtime, err := NewPolicyRuntime(store, nil)
	require.NoError(t, err)
	require.Equal(t, RuntimeUnavailable, runtime.State().Status)
	require.ErrorIs(t, runtime.Ready(), ErrAuthorizationUnavailable)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Run(ctx) }()
	synctest.Wait()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			require.ErrorIs(t, <-done, context.Canceled)
			require.False(t, runtime.State().Running)
			require.ErrorIs(t, runtime.Ready(), ErrAuthorizationUnavailable)
		})
	}
	t.Cleanup(stop)
	return runtime, stop
}

func TestPolicyRuntimeAvailability(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var unconfigured *PolicyRuntime
		require.Equal(t, RuntimeNotConfigured, unconfigured.State().Status)
		require.Empty(t, unconfigured.State().LastKnownMode)
		require.ErrorIs(t, unconfigured.Ready(), ErrAuthorizationUnavailable)
		require.ErrorIs(t, unconfigured.Mutate(t.Context(), policyActor(), readerMutation()), ErrAuthorizationUnavailable)

		db := &runtimeDatabase{Client: inmemory.NewClient()}
		store := newTestPolicyStore(t, db, true)
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		require.Equal(t, RuntimeFresh, runtime.State().Status)
		require.Equal(t, api.UserAuthorizationModeDisabled, runtime.State().LastKnownMode)
		require.Equal(t, int64(0), runtime.State().Generation)
		require.NoError(t, runtime.Ready())

		failure := errors.New("database unavailable")
		db.setGet(func(context.Context, string) error { return failure })
		for range 5 {
			time.Sleep(PolicyRefreshInterval)
			synctest.Wait()
			require.Equal(t, RuntimeFresh, runtime.State().Status)
			require.ErrorIs(t, runtime.State().LastError, failure)
		}
		time.Sleep(PolicyRefreshInterval - time.Nanosecond)
		require.Equal(t, RuntimeFresh, runtime.State().Status)
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		require.Equal(t, RuntimeUnavailable, runtime.State().Status)
		require.Equal(t, api.UserAuthorizationModeDisabled, runtime.State().LastKnownMode)
		decisions, err := runtime.Evaluate(testIdentity(), []Check{{"Radius.Core/applications/write", appA}})
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
		require.Nil(t, decisions)
		require.ErrorIs(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()), ErrAuthorizationUnavailable)

		db.setGet(nil)
		time.Sleep(PolicyRefreshInterval)
		synctest.Wait()
		require.Equal(t, RuntimeFresh, runtime.State().Status)
		require.Nil(t, runtime.State().LastError)
		decisions, err = runtime.Evaluate(testIdentity(), []Check{{"Radius.Core/applications/write", appA}})
		require.NoError(t, err)
		require.True(t, decisions[0].Allowed)
	})
}

func TestPolicyRuntimeMissingAnchor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newTestPolicyStore(t, inmemory.NewClient(), false)
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		for range 3 {
			require.Equal(t, RuntimeUnavailable, runtime.State().Status)
			require.Empty(t, runtime.State().LastKnownMode)
			require.Equal(t, int64(-1), runtime.State().Generation)
			require.ErrorIs(t, runtime.State().LastError, ErrAuthorizationUnavailable)
			require.ErrorIs(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()), ErrAuthorizationUnavailable)
			time.Sleep(PolicyRefreshInterval)
			synctest.Wait()
		}
		// Only the separate trusted initialization boundary can recover this
		// prerequisite. Starting/retrying the runtime has not created an anchor.
		initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
		require.NoError(t, err)
		require.NoError(t, initializer.Initialize(t.Context(), initialPolicy()))
		time.Sleep(PolicyRefreshInterval)
		synctest.Wait()
		require.Equal(t, RuntimeFresh, runtime.State().Status)
	})
}

func TestPolicyRuntimeStartupRecoversPendingIntent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := &policyHookClient{Client: inmemory.NewClient()}
		store := newTestPolicyStore(t, db, true)
		failure := errors.New("interrupted pending commit")
		db.after = func(_ context.Context, obj *database.Object) error {
			if saveStage(obj) == "claim" {
				return failure
			}
			return nil
		}
		_, err := store.Mutate(t.Context(), policyActor(), readerMutation())
		require.ErrorIs(t, err, failure)
		db.after = nil
		var recoveryBlocked atomic.Bool
		recoveryBlocked.Store(true)
		db.before = func(_ context.Context, obj *database.Object) error {
			if saveStage(obj) == "history" && recoveryBlocked.Load() {
				return failure
			}
			return nil
		}
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		require.Equal(t, RuntimeUnavailable, runtime.State().Status)
		require.ErrorIs(t, runtime.State().LastError, failure)
		require.Empty(t, runtime.State().LastKnownMode)
		recoveryBlocked.Store(false)
		time.Sleep(PolicyRefreshInterval)
		synctest.Wait()
		require.NoError(t, runtime.Ready())
		require.Equal(t, int64(1), runtime.State().Generation)
		history, err := store.AccessChange(t.Context(), 1)
		require.NoError(t, err)
		require.Equal(t, int64(1), *history.Properties.Generation)
	})
}

func TestPolicyRuntimeLocalAndRemoteChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := newTestPolicyStore(t, inmemory.NewClient(), true)
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		require.NoError(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()))
		require.Equal(t, int64(1), runtime.State().Generation)
		require.NoError(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()))
		require.Equal(t, int64(1), runtime.State().Generation)
		_, err := store.Mutate(t.Context(), policyActor(), assignmentMutation("remote", builtInID("reader"), teamA, "carol"))
		require.NoError(t, err)
		require.Equal(t, int64(1), runtime.State().Generation)
		time.Sleep(PolicyRefreshInterval)
		synctest.Wait()
		require.Equal(t, int64(2), runtime.State().Generation)
	})
}

func TestPolicyRuntimeRefreshFailureAfterCommit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := &runtimeDatabase{Client: inmemory.NewClient()}
		writes := &policyHookClient{Client: db}
		store := newTestPolicyStore(t, writes, true)
		runtime, stop := startPolicyRuntime(t, store)
		defer stop()
		anchorReads := 0
		failure := errors.New("local refresh failed")
		writes.after = func(_ context.Context, obj *database.Object) error {
			if saveStage(obj) == "finalize" {
				db.setGet(func(_ context.Context, id string) error {
					if id == accessVersionID {
						anchorReads++
						// Mutate completes its stable-version reread first;
						// the subsequent local refresh must fail explicitly.
						if anchorReads > 1 {
							return failure
						}
					}
					return nil
				})
			}
			return nil
		}
		err := runtime.Mutate(t.Context(), policyActor(), readerMutation())
		require.ErrorIs(t, err, ErrAuthorizationUnavailable)
		require.ErrorIs(t, err, failure)
		require.ErrorContains(t, err, "policy committed")
		require.Equal(t, int64(0), runtime.State().Generation)
		require.ErrorIs(t, runtime.State().LastError, failure)
		db.setGet(nil)
		require.NoError(t, runtime.Refresh(t.Context()))
		require.Equal(t, int64(1), runtime.State().Generation)
	})
}

func TestPolicyRuntimeCancellationAndDeadline(t *testing.T) {
	for _, block := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "blocked read"}[block], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := &runtimeDatabase{Client: inmemory.NewClient()}
				store := newTestPolicyStore(t, db, true)
				if block {
					db.setGet(func(ctx context.Context, _ string) error {
						<-ctx.Done()
						return ctx.Err()
					})

				}
				runtime, stop := startPolicyRuntime(t, store)
				require.ErrorContains(t, runtime.Run(t.Context()), "only run once")
				if block {
					time.Sleep(PolicyRefreshInterval)
					synctest.Wait()
					require.ErrorIs(t, runtime.State().LastError, context.DeadlineExceeded)
				}
				stop()
				require.ErrorIs(t, runtime.Refresh(t.Context()), ErrAuthorizationUnavailable)
				decisions, err := runtime.Evaluate(testIdentity(), nil)
				require.ErrorIs(t, err, ErrAuthorizationUnavailable)
				require.Nil(t, decisions)
			})
		})
	}
}

func TestPolicyRuntimeCancelsLocalRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := &runtimeDatabase{Client: inmemory.NewClient()}
		store := newTestPolicyStore(t, db, true)
		runtime, stop := startPolicyRuntime(t, store)
		db.setGet(func(ctx context.Context, _ string) error {
			<-ctx.Done()
			return ctx.Err()
		})
		done := make(chan error, 1)
		go func() { done <- runtime.Refresh(context.Background()) }()
		synctest.Wait()
		stop()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestPolicyRuntimeModeDoesNotBypassFreshness(t *testing.T) {
	for _, mode := range []api.UserAuthorizationMode{
		api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce,
	} {
		t.Run(string(mode), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := &runtimeDatabase{Client: inmemory.NewClient()}
				store := newTestPolicyStore(t, db, false)
				policy := initialPolicy()
				policy.Settings.Properties.Mode = &mode
				initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
				require.NoError(t, err)
				require.NoError(t, initializer.Initialize(t.Context(), policy))
				runtime, stop := startPolicyRuntime(t, store)
				defer stop()
				require.Equal(t, mode, runtime.State().LastKnownMode)
				require.NoError(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()))
				require.Equal(t, int64(1), runtime.State().Generation)
				db.setGet(func(context.Context, string) error { return errors.New("unavailable") })
				time.Sleep(PolicyMaxAge)
				synctest.Wait()
				require.ErrorIs(t, runtime.Ready(), ErrAuthorizationUnavailable)
				require.ErrorIs(t, runtime.Mutate(t.Context(), policyActor(), readerMutation()), ErrAuthorizationUnavailable)
				decisions, err := runtime.Evaluate(testIdentity(), nil)
				require.ErrorIs(t, err, ErrAuthorizationUnavailable)
				require.Nil(t, decisions)
			})
		})
	}
}
