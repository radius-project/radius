// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"k8s.io/utils/clock"
)

const (
	// PolicyRefreshInterval bounds the normal interval between version checks.
	PolicyRefreshInterval = 5 * time.Second
	// PolicyMaxAge is an exclusive upper bound on a usable policy's age.
	PolicyMaxAge = 30 * time.Second
)

// ErrAuthorizationUnavailable signals that no sufficiently fresh policy exists.
var ErrAuthorizationUnavailable = errors.New("AuthorizationUnavailable")

// Policy is the complete committed policy. Built-in roles are synthesized from
// the trusted catalog, not loaded as custom definitions from the database.
type Policy struct {
	Generation  int64
	CustomRoles []api.RoleDefinitionResource
	Assignments []api.RoleAssignmentResource
	Settings    api.AuthorizationSettingsResource
}

// PolicySnapshot owns all its data and can be shared across concurrent readers.
// Callers cannot obtain the evaluator or any mutable backing slices.
type PolicySnapshot struct {
	policy      Policy
	evaluator   *Evaluator
	roles       map[string]role
	assignments map[string]map[principalKey][]int
}

// NewPolicySnapshot validates and copies a complete, committed policy.
func NewPolicySnapshot(catalog *Catalog, policy Policy) (*PolicySnapshot, error) {
	if policy.Generation < 0 {
		return nil, fmt.Errorf("policy generation must not be negative")
	}
	owned, err := clonePolicyValue(policy)
	if err != nil {
		return nil, err
	}
	evaluator, err := NewEvaluator(catalog, owned.CustomRoles, owned.Assignments)
	if err != nil {
		return nil, err
	}
	if owned.Settings.Properties == nil || owned.Settings.Properties.Mode == nil {
		return nil, fmt.Errorf("authorization settings mode is required")
	}
	switch *owned.Settings.Properties.Mode {
	case api.UserAuthorizationModeDisabled, api.UserAuthorizationModeDryRun, api.UserAuthorizationModeEnforce:
	default:
		return nil, fmt.Errorf("unknown authorization mode")
	}
	snapshot := &PolicySnapshot{
		policy: owned, evaluator: evaluator,
		roles: map[string]role{}, assignments: map[string]map[principalKey][]int{},
	}
	definitions := append(catalog.BuiltInRoles(), owned.CustomRoles...)
	for i, definition := range definitions {
		compiled, err := compileRole(catalog, definition, i < len(definitions)-len(owned.CustomRoles))
		if err != nil {
			return nil, err
		}
		snapshot.roles[compiled.id] = compiled
	}
	for i, value := range evaluator.assignments {
		if snapshot.assignments[value.scope] == nil {
			snapshot.assignments[value.scope] = map[principalKey][]int{}
		}
		snapshot.assignments[value.scope][value.principal] = append(snapshot.assignments[value.scope][value.principal], i)
	}
	return snapshot, nil
}

func clonePolicyValue[T any](value T) (T, error) {
	var result T
	raw, err := json.Marshal(value)
	if err != nil {
		return result, fmt.Errorf("encode policy: %w", err)
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("decode policy: %w", err)
	}
	return result, nil
}

// Generation identifies the committed policy used by this snapshot.
func (s *PolicySnapshot) Generation() int64 { return s.policy.Generation }

// Policy returns a defensive copy for management and inspection, not mutation.
func (s *PolicySnapshot) Policy() (Policy, error) { return clonePolicyValue(s.policy) }

// Evaluate checks against this immutable snapshot. Runtime callers should use
// PolicyCache.Evaluate, which additionally enforces the freshness boundary.
func (s *PolicySnapshot) Evaluate(identity Identity, checks []Check) ([]Decision, error) {
	principal, err := key(&identity.Principal)
	if err != nil {
		return nil, err
	}
	principals := []principalKey{principal}
	for _, group := range identity.Groups {
		principal, err := key(&group)
		if err != nil {
			return nil, err
		}
		principals = append(principals, principal)
	}
	positions := map[int]bool{}
	for _, check := range checks {
		target, err := canonicalTarget(check.Target)
		if err != nil {
			return nil, err
		}
		for scope := target; ; {
			for _, principal := range principals {
				for _, position := range s.assignments[scope][principal] {
					positions[position] = true
				}
			}
			if scope == "/" {
				break
			}
			index := strings.LastIndex(scope, "/")
			scope = scope[:index]
			if scope == "" {
				scope = "/"
			}
		}
	}
	ordered := make([]int, 0, len(positions))
	for position := range positions {
		ordered = append(ordered, position)
	}
	slices.Sort(ordered)
	selected := Evaluator{catalog: s.evaluator.catalog}
	for _, position := range ordered {
		selected.assignments = append(selected.assignments, s.evaluator.assignments[position])
	}
	// Keep identity/group validation and deterministic explanations in the
	// shared evaluator, but scan only indexed candidate grants.
	return selected.Evaluate(identity, checks)
}

// PolicySource must recover pending changes before checking the committed
// version. It returns nil only when knownGeneration is still current; -1 forces
// a full load. All returned snapshots must use a trusted registration catalog.
type PolicySource interface {
	LoadPolicy(context.Context, int64) (*PolicySnapshot, error)
}

// PolicyCache has no background goroutines until its owner calls Run. Refresh
// also supplies the immediate refresh hook for the common local write path.
type PolicyCache struct {
	source PolicySource
	clock  clock.WithTicker
	gate   chan struct{}
	mu     sync.RWMutex
	value  *PolicySnapshot
	asOf   time.Time
}

// NewPolicyCache creates an initially unavailable cache. A nil clock uses real
// time; tests can supply a fake clock without sleeping.
func NewPolicyCache(source PolicySource, clk clock.WithTicker) (*PolicyCache, error) {
	if source == nil {
		return nil, fmt.Errorf("policy source is required")
	}
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &PolicyCache{source: source, clock: clk, gate: make(chan struct{}, 1)}, nil
}

// Refresh serializes local reloads without preventing cancellation while waiting.
// The timestamp is from before I/O: a slow read cannot extend the stale window.
func (c *PolicyCache) Refresh(ctx context.Context) error {
	select {
	case c.gate <- struct{}{}:
		defer func() { <-c.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	started := c.clock.Now()
	c.mu.RLock()
	previous := c.value
	c.mu.RUnlock()
	generation := int64(-1)
	if previous != nil {
		generation = previous.Generation()
	}
	next, err := c.source.LoadPolicy(ctx, generation)
	if err != nil {
		return fmt.Errorf("refresh policy: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if next == nil {
		next = previous
	}
	if next == nil || next.Generation() < generation || c.clock.Since(started) >= PolicyMaxAge {
		return fmt.Errorf("%w: missing, regressed, or slow policy refresh", ErrAuthorizationUnavailable)
	}
	c.mu.Lock()
	c.value, c.asOf = next, started
	c.mu.Unlock()
	return nil
}

// Ready reports whether authorization can use the cache now. A failed refresh
// never renews freshness, including when the stored generation has not changed.
func (c *PolicyCache) Ready() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.value == nil || c.clock.Since(c.asOf) >= PolicyMaxAge {
		return ErrAuthorizationUnavailable
	}
	return nil
}

// Snapshot returns an immutable snapshot only within the cache freshness bound.
func (c *PolicyCache) Snapshot() (*PolicySnapshot, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.value == nil || c.clock.Since(c.asOf) >= PolicyMaxAge {
		return nil, ErrAuthorizationUnavailable
	}
	return c.value, nil
}

// Evaluate refuses to decide at or beyond the thirty-second boundary.
func (c *PolicyCache) Evaluate(identity Identity, checks []Check) ([]Decision, error) {
	c.mu.RLock()
	value, asOf := c.value, c.asOf
	c.mu.RUnlock()
	if value == nil || c.clock.Since(asOf) >= PolicyMaxAge {
		return nil, ErrAuthorizationUnavailable
	}
	return value.Evaluate(identity, checks)
}

// Run refreshes every five seconds until cancelled. The owner must join this
// call at shutdown. Failures are delivered to report, never silently discarded.
func (c *PolicyCache) Run(ctx context.Context, report func(error)) error {
	if report == nil {
		return fmt.Errorf("policy refresh error reporter is required")
	}
	ticker := c.clock.NewTicker(PolicyRefreshInterval)
	defer ticker.Stop()
	for {
		if err := c.Refresh(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			report(err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C():
		}
	}
}
