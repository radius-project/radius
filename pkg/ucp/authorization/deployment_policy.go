// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/components/database"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
)

type deploymentReceipt struct {
	Key, ExecutionID, Fingerprint string
	Generation                    int64
}

func deploymentReceiptID(execution string) string {
	hash := sha256.Sum256([]byte(execution))
	return "/providers/System.Authorization/policyDeployments/" + hex.EncodeToString(hash[:])
}

// ValidateDeployment simulates the entire incremental policy change set. Every
// permission check uses the original policy, never a grant introduced by the plan.
func (s *PolicyStore) ValidateDeployment(ctx context.Context, snapshot *PolicySnapshot, actor PolicyActor, mutations []PolicyMutation) error {
	_, _, _, err := s.simulateDeployment(ctx, snapshot, actor, mutations)
	return err
}

func (s *PolicyStore) simulateDeployment(ctx context.Context, original *PolicySnapshot, actor PolicyActor, mutations []PolicyMutation) (Policy, []*api.PolicyChange, []bool, error) {
	if original == nil || len(mutations) == 0 || len(mutations) > 100 ||
		actor.Management.Kind == nil || *actor.Management.Kind != api.AssignmentManagementKindDeployment {
		return Policy{}, nil, nil, fmt.Errorf("Invalid: deployment requires 1-100 policy resources and verified deployment provenance")
	}
	input, err := clonePolicyValue(mutations)
	if err != nil {
		return Policy{}, nil, nil, err
	}
	// Definitions must exist in the simulated policy before their assignments.
	slices.SortStableFunc(input, func(a, b PolicyMutation) int {
		if (a.Role != nil) != (b.Role != nil) {
			if a.Role != nil {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Target), strings.ToLower(b.Target))
	})
	working := original
	seen := map[string]bool{}
	changes := []*api.PolicyChange{}
	impacts := []bool{}
	var result Policy
	for _, mutation := range input {
		target, err := canonicalTarget(mutation.Target)
		if err != nil {
			return Policy{}, nil, nil, err
		}
		if seen[target] || mutation.Delete || mutation.Settings != nil ||
			(mutation.Role == nil) == (mutation.Assignment == nil) {
			return Policy{}, nil, nil, fmt.Errorf("Invalid: incremental deployment accepts distinct role definitions and assignments only")
		}
		seen[target] = true
		var change *api.PolicyChange
		var impact bool
		result, change, impact, err = applyPolicyMutationWithAuthority(ctx, working, original, actor, mutation, s.settings)
		if err != nil {
			return Policy{}, nil, nil, err
		}
		working, err = NewPolicySnapshot(s.catalog, result)
		if err != nil {
			return Policy{}, nil, nil, err
		}
		if change != nil {
			changes = append(changes, change)
			impacts = append(impacts, impact)
		}
	}
	return result, changes, impacts, nil
}

// ApplyDeployment atomically publishes a previously approved policy plan. Its
// receipt is committed with policy/history, so concurrent engine retries cannot
// replay the plan after another writer changes policy.
func (s *PolicyStore) ApplyDeployment(ctx context.Context, execution string, generation int64, actor PolicyActor, mutations []PolicyMutation) (*PolicySnapshot, error) {
	if execution == "" {
		return nil, fmt.Errorf("Invalid: execution record is required")
	}
	raw, err := json.Marshal(struct {
		Generation int64
		Actor      PolicyActor
		Mutations  []PolicyMutation
	}{generation, actor, mutations})
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	key := deploymentReceiptID(execution)
	budget := &policyRetryBudget{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		version, object, err := s.stableVersion(ctx, budget)
		if err != nil {
			return nil, err
		}
		var receipt deploymentReceipt
		_, err = s.read(ctx, key, &receipt)
		if err == nil {
			if receipt.Fingerprint != fingerprint || receipt.ExecutionID != execution || receipt.Generation > version.Generation {
				return nil, fmt.Errorf("AuthorizationFailed: execution record policy plan differs from its committed receipt")
			}
			return s.loadPolicy(ctx, -1, budget)
		}
		if !errors.Is(err, &database.ErrNotFound{}) {
			return nil, err
		}
		if version.Generation != generation {
			return nil, fmt.Errorf("%w: policy changed after deployment preflight; rerun the deployment", ErrPolicyConflict)
		}
		stored, err := s.loadRoot(ctx, version.Root, version.Generation)
		if err != nil {
			return nil, err
		}
		current, err := NewPolicySnapshot(s.catalog, stored.policy)
		if err != nil {
			return nil, err
		}
		next, changes, impacts, err := s.simulateDeployment(ctx, current, actor, mutations)
		if err != nil {
			return nil, err
		}
		if len(changes) == 0 {
			return current, nil
		}
		if int64(len(changes)) > maxGeneration-version.Generation {
			return nil, fmt.Errorf("PolicyCapacityExceeded: generation space exhausted")
		}
		next.Generation = version.Generation + int64(len(changes))
		next.Settings.Properties.Generation = to.Ptr(next.Generation)
		intent := uuid.NewString()
		root, err := s.prepare(ctx, intent, next, stored)
		if err != nil {
			return nil, err
		}
		events := make([]string, len(changes))
		for index, change := range changes {
			number := version.Generation + int64(index) + 1
			event := api.AccessChangeResource{
				ID: to.Ptr(historyPrefix + strconv.FormatInt(number, 10)),
				Properties: &api.AccessChangeProperties{
					Generation: &number, HighImpact: &impacts[index], Mode: next.Settings.Properties.Mode,
					PolicyChange: change, Principal: &actor.Identity.Principal, Recovery: to.Ptr(actor.Recovery),
					RequestID: &actor.RequestID, CorrelationID: &actor.CorrelationID, Time: to.Ptr(s.clock.Now().UTC()),
				},
			}
			events[index] = policyObjectID + intent + "-event-" + strconv.Itoa(index)
			if err := s.immutable(ctx, events[index], event); err != nil {
				return nil, err
			}
		}
		receipt = deploymentReceipt{Key: key, ExecutionID: execution, Fingerprint: fingerprint, Generation: next.Generation}
		reference := policyObjectID + intent + "-receipt"
		if err := s.immutable(ctx, reference, receipt); err != nil {
			return nil, err
		}
		version.Pending = &pendingPolicy{Generation: next.Generation, Intent: intent, Root: root, Events: events, Receipt: reference}
		if err := s.save(ctx, accessVersionID, version, database.WithETag(object.ETag)); err != nil {
			var conflict *database.ErrConcurrency
			if errors.As(err, &conflict) {
				if err := budget.retry(); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		return s.loadPolicy(ctx, -1, budget)
	}
}
