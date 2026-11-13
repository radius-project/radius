// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/google/uuid"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	api "github.com/radius-project/radius/pkg/ucp/api/authorization/v20250801preview"
	"github.com/stretchr/testify/require"
)

func largeHistoryMutation() PolicyMutation {
	mutation := readerMutation()
	mutation.Assignment.Description = to.Ptr(strings.Repeat("x", 130500))
	return mutation
}

func TestPolicyStoreChunkedHistoryRecovery(t *testing.T) {
	runPolicyStoreMutationCrashRecovery(t, func(*testing.T) database.Client { return inmemory.NewClient() },
		largeHistoryMutation(), []string{"chunk-0", "chunk-1", "chunk-2", "event", "claim", "history", "finalize"})
}

func TestPolicyStoreChunkedHistoryDelayedRecovery(t *testing.T) {
	runPolicyStoreMutationDelayedRecovery(t, inmemory.NewClient(), largeHistoryMutation())
}

func TestPolicyStoreChunkedHistoryEventReferences(t *testing.T) {
	for _, suffix := range []string{"", "-0", "-99", "-01", "--1", "-100", "-1junk", "junk", "-0-event"} {
		t.Run("event"+suffix, func(t *testing.T) {
			t.Parallel()
			store := newTestPolicyStore(t, inmemory.NewClient(), true)
			_, err := store.Mutate(t.Context(), policyActor(), largeHistoryMutation())
			require.NoError(t, err)
			event, err := store.AccessChange(t.Context(), 1)
			require.NoError(t, err)
			id := policyObjectID + uuid.NewString() + "-event" + suffix
			require.NoError(t, store.prepareHistory(t.Context(), id, *event))
			actual, record, err := store.readHistory(t.Context(), id)
			if suffix != "" && suffix != "-0" && suffix != "-99" {
				require.ErrorContains(t, err, "invalid access history chunk prefix")
				return
			}
			require.NoError(t, err)
			require.Equal(t, event, actual)
			_, err = store.Mutate(t.Context(), policyActor(), readerMutation())
			require.NoError(t, err)
			require.NoError(t, store.save(t.Context(), historyPrefix+"2", record))
			_, err = store.AccessChange(t.Context(), 2)
			require.ErrorContains(t, err, "access history generation mismatch")
		})
	}
}

func TestPolicyStoreChunkedHistoryIntegrity(t *testing.T) {
	for _, stage := range []string{"pending", "published"} {
		for _, scenario := range []string{"missing chunk", "duplicate", "foreign prefix", "invalid intent", "out of order", "empty", "oversize", "short chunk", "invalid JSON", "valid JSON corruption", "wrong generation", "too many", "wrong size", "wrong digest"} {
			t.Run(stage+"/"+scenario, func(t *testing.T) {
				db := inmemory.NewClient()
				writer := newTestPolicyStore(t, db, true)
				wrapped := &policyHookClient{Client: db, before: func(_ context.Context, object *database.Object) error {
					raw, err := json.Marshal(object.Data)
					require.NoError(t, err)
					require.LessOrEqual(t, len(raw), maxPolicyBytes, "every backend object stays bounded")
					return nil
				}}
				crash := errors.New("stop after claiming intent")
				if stage == "pending" {
					wrapped.after = func(_ context.Context, object *database.Object) error {
						if saveStage(object) == "claim" {
							return crash
						}
						return nil
					}
				}
				writer.db = wrapped
				_, err := writer.Mutate(t.Context(), policyActor(), largeHistoryMutation())
				id := historyPrefix + "1"
				if stage == "pending" {
					require.ErrorIs(t, err, crash)
					var version accessVersion
					_, err = writer.read(t.Context(), accessVersionID, &version)
					require.NoError(t, err)
					id = version.Pending.Event
				} else {
					require.NoError(t, err)
				}
				var manifest historyManifest
				_, err = writer.read(t.Context(), id, &manifest)
				require.NoError(t, err)
				require.Len(t, manifest.Chunks, 3)
				switch scenario {
				case "missing chunk":
					require.NoError(t, db.Delete(t.Context(), manifest.Chunks[2]))
				case "duplicate":
					manifest.Chunks[1] = manifest.Chunks[0]
				case "foreign prefix":
					manifest.Chunks[0] = "/providers/Radius.Core/roleAssignments/bad"
				case "invalid intent":
					manifest.Chunks[0] = policyObjectID + "invalid-event-chunk-0"
				case "out of order":
					manifest.Chunks[0], manifest.Chunks[1] = manifest.Chunks[1], manifest.Chunks[0]
				case "too many":
					manifest.Chunks = make([]string, maxHistoryChunks+1)
				case "wrong size":
					manifest.Size--
				case "wrong digest":
					manifest.SHA256 = strings.Repeat("0", 64)
				default:
					var chunk historyChunk
					_, err = writer.read(t.Context(), manifest.Chunks[0], &chunk)
					require.NoError(t, err)
					switch scenario {
					case "empty":
						chunk.Data = nil
					case "oversize":
						chunk.Data = make([]byte, historyChunkBytes+1)
					case "short chunk":
						chunk.Data = chunk.Data[:len(chunk.Data)-1]
					case "invalid JSON":
						chunk.Data[0] = '!'
					case "wrong generation":
						chunk.Data = []byte(strings.Replace(string(chunk.Data), `"generation":1`, `"generation":9`, 1))
					case "valid JSON corruption":
						chunk.Data = []byte(strings.Replace(string(chunk.Data), `"description":"x`, `"description":"y`, 1))
					}
					require.NoError(t, writer.save(t.Context(), manifest.Chunks[0], chunk))
				}
				require.NoError(t, writer.save(t.Context(), id, manifest))
				if stage == "pending" {
					_, err = writer.LoadPolicy(t.Context(), -1)
				} else {
					_, err = writer.AccessChange(t.Context(), 1)
				}
				require.Error(t, err)
				if stage == "pending" {
					var version accessVersion
					_, err = writer.read(t.Context(), accessVersionID, &version)
					require.NoError(t, err)
					require.Zero(t, version.Generation)
					require.NotNil(t, version.Pending, "corrupt audit cannot publish prepared grants")
					_, err = db.Get(t.Context(), historyPrefix+"1")
					var missing *database.ErrNotFound
					require.ErrorAs(t, err, &missing)
				}
			})
		}
	}
}

func TestPolicyStoreHistoryChunkBound(t *testing.T) {
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	event, err := store.Mutate(t.Context(), policyActor(), readerMutation())
	require.NoError(t, err)
	require.Equal(t, int64(1), event.Generation())
	// Legacy inline history remains readable without a manifest.
	history, err := store.AccessChange(t.Context(), 1)
	require.NoError(t, err)
	history.Properties.RequestID = to.Ptr(strings.Repeat("x", maxHistoryChunks*historyChunkBytes))
	require.ErrorContains(t, store.prepareHistory(t.Context(), policyObjectID+"bound-event", *history), "PolicyCapacityExceeded")
	var record map[string]json.RawMessage
	_, err = store.read(t.Context(), historyPrefix+"1", &record)
	require.NoError(t, err)
	require.Contains(t, record, "properties")
	require.NotContains(t, record, "historyChunks")
}

func TestPolicyStoreHistoryMetadataHeadroom(t *testing.T) {
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	actor := policyActor()
	_, err := store.Mutate(t.Context(), actor, readerMutation())
	require.NoError(t, err)
	event, err := store.AccessChange(t.Context(), 1)
	require.NoError(t, err)
	metadata := api.AccessChangeResource{Properties: &api.AccessChangeProperties{
		Principal: event.Properties.Principal, RequestID: event.Properties.RequestID, CorrelationID: event.Properties.CorrelationID,
		PolicyChange: &api.PolicyChange{Target: event.Properties.PolicyChange.Target, ManagedBy: event.Properties.PolicyChange.ManagedBy},
	}}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	actor.RequestID += strings.Repeat("r", maxHistoryMetadataBytes-len(raw))
	mutation := largeHistoryMutation()
	for generation := int64(2); generation <= 9; generation++ {
		mutation.Assignment.Description = to.Ptr(strings.Repeat(string(rune('a'+generation)), 130500))
		snapshot, err := store.Mutate(t.Context(), actor, mutation)
		require.NoError(t, err)
		require.Equal(t, generation, snapshot.Generation())
	}
	snapshot, err := store.Mutate(t.Context(), actor, PolicyMutation{Target: mutation.Target, Delete: true})
	require.NoError(t, err, "max metadata still permits before-only history and an extra generation digit")
	require.Equal(t, int64(10), snapshot.Generation())
	event, err = store.AccessChange(t.Context(), 10)
	require.NoError(t, err)
	require.Equal(t, actor.RequestID, *event.Properties.RequestID)
	require.NotNil(t, event.Properties.PolicyChange.Before.RoleAssignment)
	actor.RequestID += "r"
	_, err = store.Mutate(t.Context(), actor, mutation)
	require.ErrorContains(t, err, "access history metadata exceeds byte bound")
	snapshot, err = store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	require.Equal(t, int64(10), snapshot.Generation(), "oversized metadata cannot claim a new generation")
}

func TestPolicyStoreMaximumHistorySnapshots(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("full-size history boundary; omit -short to run")
	}
	store := newTestPolicyStore(t, inmemory.NewClient(), true)
	_, err := store.Mutate(t.Context(), policyActor(), readerMutation())
	require.NoError(t, err)
	event, err := store.AccessChange(t.Context(), 1)
	require.NoError(t, err)
	event.Properties.PolicyChange.Before = &api.PolicySnapshot{RoleAssignment: &api.RoleAssignmentProperties{Description: to.Ptr("")}}
	raw, err := json.Marshal(event.Properties.PolicyChange.Before)
	require.NoError(t, err)
	event.Properties.PolicyChange.Before.RoleAssignment.Description = to.Ptr(strings.Repeat("x", maxHistorySnapshotBytes-len(raw)))
	event.Properties.PolicyChange.After = event.Properties.PolicyChange.Before
	event.Properties.RequestID = to.Ptr("")
	metadata := api.AccessChangeResource{Properties: &api.AccessChangeProperties{
		Principal: event.Properties.Principal, RequestID: event.Properties.RequestID, CorrelationID: event.Properties.CorrelationID,
		PolicyChange: &api.PolicyChange{Target: event.Properties.PolicyChange.Target, ManagedBy: event.Properties.PolicyChange.ManagedBy},
	}}
	raw, err = json.Marshal(metadata)
	require.NoError(t, err)
	event.Properties.RequestID = to.Ptr(strings.Repeat("r", maxHistoryMetadataBytes-len(raw)))
	id := policyObjectID + uuid.NewString() + "-event"
	written := 0
	store.db = &policyHookClient{Client: store.db, before: func(_ context.Context, object *database.Object) error {
		raw, err := json.Marshal(object.Data)
		require.NoError(t, err)
		require.LessOrEqual(t, len(raw), maxPolicyBytes)
		written++
		return nil
	}}
	require.NoError(t, store.prepareHistory(t.Context(), id, *event))
	require.LessOrEqual(t, written, maxHistoryChunks+1)
	require.Greater(t, written, maxHistoryChunks-2)
	actual, _, err := store.readHistory(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, event, actual, "reassembly returns the public AccessChange unchanged")
	event.Properties.PolicyChange.Before.RoleAssignment.Description = to.Ptr(*event.Properties.PolicyChange.Before.RoleAssignment.Description + "x")
	require.ErrorContains(t, store.prepareHistory(t.Context(), policyObjectID+uuid.NewString()+"-event", *event), "snapshot exceeds byte bound")
}

func TestPolicyStoreBootstrapHistoryCapacity(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("full-size compound bootstrap history; omit -short to run")
	}
	db := inmemory.NewClient()
	store := newTestPolicyStore(t, db, false)
	policy := initialPolicy()
	policy.Assignments = nil
	initializer, err := NewPolicyInitializer(store, initializationVerifierFunc(func(context.Context, Policy) error { return nil }))
	require.NoError(t, err)
	require.NoError(t, initializer.Initialize(t.Context(), policy))
	actor := policyActor()
	actor.RequestID = strings.Repeat("r", maxHistoryMetadataBytes-1024)
	principal := testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", "bootstrap-group-000")
	principal.DisplayName = to.Ptr("")
	require.NoError(t, addBootstrapAdministrators(&policy, []api.Principal{principal}, actor))
	assignment := policy.Assignments[0]
	encoded, err := json.Marshal(assignment)
	require.NoError(t, err)
	payload, err := json.Marshal(policyPayload{Entries: map[string]json.RawMessage{*assignment.ID: encoded}})
	require.NoError(t, err)
	label := strings.Repeat("x", maxPolicyBytes-len(payload))
	guard := &bootstrapSettingsGuard{}
	for i := 0; i < manifestFanout-1; i++ {
		group := testPrincipal(api.PrincipalTypeGroup, "kubernetes:cluster-a", fmt.Sprintf("bootstrap-group-%03d", i))
		group.DisplayName = &label
		guard.principals = append(guard.principals, group)
	}
	store.settings = guard
	crash := errors.New("stop large compound publication")
	store.db = &policyHookClient{Client: db, before: func(_ context.Context, object *database.Object) error {
		raw, err := json.Marshal(object.Data)
		require.NoError(t, err)
		require.LessOrEqual(t, len(raw), maxPolicyBytes)
		if saveStage(object) == "finalize" {
			return crash
		}
		return nil
	}}
	mutation := PolicyMutation{Target: settingsID, Settings: &api.AuthorizationSettingsProperties{Mode: to.Ptr(api.UserAuthorizationModeDryRun)}}
	_, err = store.Mutate(t.Context(), actor, mutation)
	require.ErrorIs(t, err, crash)
	store = newTestPolicyStore(t, db, false)
	store.settings = guard
	snapshot, err := store.LoadPolicy(t.Context(), -1)
	require.NoError(t, err)
	actual, err := snapshot.Policy()
	require.NoError(t, err)
	require.Equal(t, int64(1), actual.Generation)
	require.Len(t, actual.Assignments, manifestFanout-1)
	var version accessVersion
	_, err = store.read(t.Context(), accessVersionID, &version)
	require.NoError(t, err)
	var root manifestRoot
	_, err = store.read(t.Context(), version.Root, &root)
	require.NoError(t, err)
	require.Len(t, root.Pages, manifestFanout, "each maximum-sized assignment plus settings consumes one payload")
	event, err := store.AccessChange(t.Context(), 1)
	require.NoError(t, err)
	require.Equal(t, actor.RequestID, *event.Properties.RequestID)
	require.Empty(t, event.Properties.PolicyChange.Before.RoleAssignments)
	require.Len(t, event.Properties.PolicyChange.After.RoleAssignments, manifestFanout-1)
	for _, added := range event.Properties.PolicyChange.After.RoleAssignments {
		require.Contains(t, actual.Assignments, *added)
	}
	// One more full-size grant exceeds policy capacity, not the audit budget.
	principal.Subject = to.Ptr("bootstrap-group-127")
	principal.DisplayName = &label
	guard.principals = append(guard.principals, principal)
	_, err = store.Mutate(t.Context(), actor, mutation)
	require.ErrorContains(t, err, "PolicyCapacityExceeded")
	// A nil snapshot for known generation 1 proves no generation was claimed without re-decoding full policy.
	snapshot, err = store.LoadPolicy(t.Context(), 1)
	require.NoError(t, err)
	require.Nil(t, snapshot)
}

func TestPolicyStoreLargeAssignmentHistory(t *testing.T) {
	for _, operation := range []string{"delete", "clear", "replace"} {
		t.Run(operation, func(t *testing.T) {
			store := newTestPolicyStore(t, inmemory.NewClient(), true)
			mutation := readerMutation()
			mutation.Assignment.Description = to.Ptr(strings.Repeat("x", 130284))
			_, err := store.Mutate(t.Context(), policyActor(), mutation)
			require.NoError(t, err)
			before := *mutation.Assignment.Description
			if operation == "delete" {
				mutation = PolicyMutation{Target: mutation.Target, Delete: true}
			} else if operation == "clear" {
				mutation.Assignment.Description = nil
			} else {
				mutation.Assignment.Description = to.Ptr(strings.Repeat("y", 130284))
			}
			_, err = store.Mutate(t.Context(), policyActor(), mutation)
			require.NoError(t, err)
			event, err := store.AccessChange(t.Context(), 2)
			require.NoError(t, err)
			require.Equal(t, before, *event.Properties.PolicyChange.Before.RoleAssignment.Description)
			if operation == "delete" {
				require.Nil(t, event.Properties.PolicyChange.After)
			} else {
				require.Equal(t, mutation.Assignment.Description, event.Properties.PolicyChange.After.RoleAssignment.Description)
			}
		})
	}
}
