// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"fmt"
	"sync"
	"time"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"k8s.io/utils/clock"
)

// RuntimeOptions enables the policy lifecycle independently of internal component
// authorization and the workload mode persisted in authorization settings.
type RuntimeOptions struct {
	Enabled bool `yaml:"enabled"`
}

// RuntimeStatus describes policy availability, not workload authorization mode.
type RuntimeStatus string

const (
	// RuntimeNotConfigured means the policy runtime has not been enabled.
	RuntimeNotConfigured RuntimeStatus = "NotConfigured"
	// RuntimeUnavailable means the runtime is starting, stale, or stopped.
	RuntimeUnavailable RuntimeStatus = "Unavailable"
	// RuntimeFresh means the runtime can currently evaluate policy.
	RuntimeFresh RuntimeStatus = "Fresh"
)

// RuntimeState is an observational copy, never an authorization decision.
// LastKnownMode is empty before the first load, and may be stale when unavailable.
// It must not be interpreted as Disabled when absent. LastError records the most
// recent refresh failure even while the previous policy remains fresh.
type RuntimeState struct {
	Status        RuntimeStatus
	Running       bool
	Generation    int64
	LastKnownMode api.UserAuthorizationMode
	AsOf          time.Time
	LastError     error
}

// PolicyRuntime owns one store/cache pair. Run is called once by the UCP host;
// it owns the blocking refresh loop and returns only after cancellation stops it.
// It never initializes a missing anchor or exposes a policy snapshot to callers.
type PolicyRuntime struct {
	store *PolicyStore
	cache *PolicyCache
	mu    sync.RWMutex
	ctx   context.Context
}

// NewPolicyRuntime connects an existing store to its freshness-checked cache.
// No I/O or goroutines occur until Run; the store must already be initialized
// through the separate trusted initialization boundary.
func NewPolicyRuntime(store *PolicyStore, clk clock.WithTicker) (*PolicyRuntime, error) {
	if store == nil {
		return nil, fmt.Errorf("policy store is required")
	}
	cache, err := NewPolicyCache(store, clk)
	if err != nil {
		return nil, err
	}
	return &PolicyRuntime{store: store, cache: cache}, nil
}

// Name identifies this service to the shared UCP host.
func (r *PolicyRuntime) Name() string { return "user-authorization-policy" }

// Run immediately recovers/loads policy, then checks every five seconds.
// Read failures, including a missing anchor, are logged and retried rather than
// terminating UCP or manufacturing a Disabled policy. No caller can use stale
// policy, but ordinary DryRun workload availability is left to the request layer.
func (r *PolicyRuntime) Run(ctx context.Context) error {
	r.mu.Lock()
	if r.ctx != nil {
		r.mu.Unlock()
		return fmt.Errorf("policy runtime can only run once")
	}
	lifetime, cancel := context.WithCancel(ctx)
	r.ctx = lifetime
	r.mu.Unlock()
	defer cancel()

	logger := ucplog.FromContextOrDiscard(ctx)
	return r.cache.Run(lifetime, func(err error) {
		logger.Error(err, "User authorization policy refresh failed")
	})
}

// State returns an explicit availability state, including on a nil (unconfigured)
// runtime. A stopped runtime is unavailable even if its cached data is young.
func (r *PolicyRuntime) State() RuntimeState {
	if r == nil {
		return RuntimeState{Status: RuntimeNotConfigured, Generation: -1}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.cache.mu.RLock()
	defer r.cache.mu.RUnlock()
	state := RuntimeState{
		Status: RuntimeUnavailable, Generation: -1,
		AsOf: r.cache.asOf, LastError: r.cache.lastError,
		Running: r.ctx != nil && r.ctx.Err() == nil,
	}
	if r.cache.value != nil {
		state.Generation = r.cache.value.Generation()
		state.LastKnownMode = *r.cache.value.policy.Settings.Properties.Mode
		if state.Running && r.cache.clock.Since(state.AsOf) < PolicyMaxAge {
			state.Status = RuntimeFresh
		}
	}
	return state
}

// Ready is authorization-specific readiness, not global pod/discovery readiness.
// Protected policy writes must require it independently of LastKnownMode.
func (r *PolicyRuntime) Ready() error {
	if r.State().Status != RuntimeFresh {
		return ErrAuthorizationUnavailable
	}
	return nil
}

// Evaluate never returns decisions when policy is unavailable or stale.
func (r *PolicyRuntime) Evaluate(identity Identity, checks []Check) ([]Decision, error) {
	if r == nil {
		return nil, ErrAuthorizationUnavailable
	}
	r.mu.RLock()
	lifetime := r.ctx
	r.mu.RUnlock()
	r.cache.mu.RLock()
	value, asOf := r.cache.value, r.cache.asOf
	r.cache.mu.RUnlock()
	if lifetime == nil || lifetime.Err() != nil || value == nil || r.cache.clock.Since(asOf) >= PolicyMaxAge {
		return nil, ErrAuthorizationUnavailable
	}
	decisions, err := value.Evaluate(identity, checks)
	// Evaluation uses an immutable snapshot without holding refresh locks. Check
	// that same snapshot's age and lifetime again before publishing any result.
	if r.cache.clock.Since(asOf) >= PolicyMaxAge || lifetime.Err() != nil {
		return nil, ErrAuthorizationUnavailable
	}
	return decisions, err
}

func (r *PolicyRuntime) operationContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if r == nil {
		return nil, nil, ErrAuthorizationUnavailable
	}
	r.mu.RLock()
	lifetime := r.ctx
	r.mu.RUnlock()
	if lifetime == nil || lifetime.Err() != nil {
		return nil, nil, ErrAuthorizationUnavailable
	}
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(lifetime, cancel)
	return ctx, func() {
		stop()
		cancel()
	}, nil
}

// Refresh immediately checks the committed generation, including recovery of
// pending intent. It is also available while stale to allow explicit recovery.
func (r *PolicyRuntime) Refresh(ctx context.Context) error {
	ctx, cancel, err := r.operationContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if err := r.cache.Refresh(ctx); err != nil {
		return fmt.Errorf("%w: %w", ErrAuthorizationUnavailable, err)
	}
	return nil
}

// Mutate requires fresh policy in every workload mode and delegates all access
// and safety checks to the existing store. Successful and idempotent writes
// immediately refresh the local cache; a failed refresh is not write success.
// The request layer must supply only verified identity and server-derived actor
// metadata. Settings changes still require the store's trusted settings guard.
func (r *PolicyRuntime) Mutate(ctx context.Context, actor PolicyActor, mutation PolicyMutation) error {
	if err := r.Ready(); err != nil {
		return err
	}
	ctx, cancel, err := r.operationContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := r.store.MutateAndRefresh(ctx, r.cache, actor, mutation); err != nil {
		return err
	}
	return nil
}
