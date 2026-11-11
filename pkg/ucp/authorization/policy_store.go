// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"bytes"
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
	"k8s.io/utils/clock"
)

const (
	accessVersionID   = "/providers/System.Authorization/accessVersions/default"
	policyObjectID    = "/providers/System.Authorization/policyObjects/"
	historyPrefix     = "/providers/Radius.Core/accessChanges/"
	maxPolicyBytes    = 128 * 1024
	manifestFanout    = 128
	maxGeneration     = int64(1<<53 - 1) // All current JSON-backed clients preserve these integers.
	policyRetries     = 5
	historyChunkBytes = maxPolicyBytes / 2
	// A snapshot can include all packed policy resources plus field/array wrappers.
	// Bound actor, request and provenance metadata separately from policy contents.
	maxHistorySnapshotBytes = (manifestFanout + 1) * maxPolicyBytes
	maxHistoryMetadataBytes = maxPolicyBytes
	maxHistoryEnvelopeBytes = maxHistoryMetadataBytes + maxPolicyBytes
	maxHistoryChunks        = (2*maxHistorySnapshotBytes + maxHistoryEnvelopeBytes) / historyChunkBytes
)

// ErrPolicyConflict means five optimistic version-conflict retries were exhausted.
var ErrPolicyConflict = errors.New("Conflict: policy version retry limit exceeded")

type accessVersion struct {
	Generation int64          `json:"generation"`
	Root       string         `json:"root"`
	Pending    *pendingPolicy `json:"pending,omitempty"`
}

type pendingPolicy struct {
	Generation int64  `json:"generation"`
	Intent     string `json:"intent"`
	Root       string `json:"root"`
	Event      string `json:"event"`
}

type manifestRoot struct {
	Generation int64    `json:"generation"`
	Pages      []string `json:"pages"`
}

type policyReference struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

type manifestPage struct {
	Object  string            `json:"object"`
	Entries []policyReference `json:"entries"`
}

type policyPayload struct {
	Entries map[string]json.RawMessage `json:"entries"`
}

type historyManifest struct {
	Chunks []string `json:"historyChunks"`
	Size   int      `json:"size"`
	SHA256 string   `json:"sha256"`
}

type historyChunk struct {
	Data []byte `json:"data"`
}

// prepareHistory preserves small, existing inline events and splits larger
// events into immutable bounded objects before the intent can be claimed.
func (s *PolicyStore) prepareHistory(ctx context.Context, id string, event api.AccessChangeResource) error {
	if event.Properties == nil || event.Properties.PolicyChange == nil {
		return fmt.Errorf("invalid access history properties")
	}
	// Bound variable metadata independently of operation, generation digit count,
	// timestamp precision, and before/after presence, so those cannot consume the
	// headroom needed to revoke an already accepted entry.
	metadata := api.AccessChangeResource{Properties: &api.AccessChangeProperties{
		Principal: event.Properties.Principal, RequestID: event.Properties.RequestID, CorrelationID: event.Properties.CorrelationID,
		PolicyChange: &api.PolicyChange{Target: event.Properties.PolicyChange.Target, ManagedBy: event.Properties.PolicyChange.ManagedBy},
	}}
	rawMetadata, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if len(rawMetadata) > maxHistoryMetadataBytes {
		return fmt.Errorf("PolicyCapacityExceeded: access history metadata exceeds byte bound")
	}
	envelope := event
	properties := *event.Properties
	change := *properties.PolicyChange
	for _, snapshot := range []*api.PolicySnapshot{change.Before, change.After} {
		if snapshot == nil {
			continue
		}
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		if len(raw) > maxHistorySnapshotBytes {
			return fmt.Errorf("PolicyCapacityExceeded: access history snapshot exceeds byte bound")
		}
	}
	if change.Before != nil {
		change.Before = &api.PolicySnapshot{}
	}
	if change.After != nil {
		change.After = &api.PolicySnapshot{}
	}
	properties.PolicyChange = &change
	envelope.Properties = &properties
	rawEnvelope, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	if len(rawEnvelope) > maxHistoryEnvelopeBytes {
		return fmt.Errorf("PolicyCapacityExceeded: access history envelope exceeds byte bound")
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if len(raw) <= maxPolicyBytes {
		return s.immutable(ctx, id, event)
	}
	if len(raw) > maxHistoryChunks*historyChunkBytes {
		return fmt.Errorf("PolicyCapacityExceeded: access history exceeds chunk bound")
	}
	checksum := sha256.Sum256(raw)
	manifest := historyManifest{Size: len(raw), SHA256: hex.EncodeToString(checksum[:])}
	for offset := 0; offset < len(raw); offset += historyChunkBytes {
		chunkID := id + "-chunk-" + strconv.Itoa(len(manifest.Chunks))
		chunk := historyChunk{Data: raw[offset:min(offset+historyChunkBytes, len(raw))]}
		if err := s.immutable(ctx, chunkID, chunk); err != nil {
			return err
		}
		manifest.Chunks = append(manifest.Chunks, chunkID)
	}
	return s.immutable(ctx, id, manifest)
}

func (s *PolicyStore) readHistory(ctx context.Context, id string) (*api.AccessChangeResource, json.RawMessage, error) {
	var record json.RawMessage
	if _, err := s.read(ctx, id, &record); err != nil {
		return nil, nil, err
	}
	var manifest historyManifest
	if err := json.Unmarshal(record, &manifest); err != nil {
		return nil, nil, err
	}
	raw := record
	if manifest.Chunks != nil {
		if len(manifest.Chunks) < 2 || len(manifest.Chunks) > maxHistoryChunks ||
			manifest.Size <= maxPolicyBytes || manifest.Size > maxHistoryChunks*historyChunkBytes ||
			len(manifest.Chunks) != (manifest.Size+historyChunkBytes-1)/historyChunkBytes ||
			len(manifest.SHA256) != sha256.Size*2 {
			return nil, nil, fmt.Errorf("invalid access history chunk count")
		}
		prefix := strings.TrimSuffix(manifest.Chunks[0], "-chunk-0")
		intent := strings.TrimSuffix(strings.TrimPrefix(prefix, policyObjectID), "-event")
		parsed, err := uuid.Parse(intent)
		if err != nil || parsed.String() != intent ||
			prefix != policyObjectID+intent+"-event" ||
			(strings.HasPrefix(id, policyObjectID) && prefix != id) {
			return nil, nil, fmt.Errorf("invalid access history chunk prefix")
		}
		raw = make([]byte, 0, manifest.Size)
		for index, chunkID := range manifest.Chunks {
			if chunkID != prefix+"-chunk-"+strconv.Itoa(index) {
				return nil, nil, fmt.Errorf("invalid access history chunk reference")
			}
			var chunk historyChunk
			if _, err := s.read(ctx, chunkID, &chunk); err != nil {
				return nil, nil, err
			}
			if len(chunk.Data) == 0 || len(chunk.Data) > historyChunkBytes ||
				(index < len(manifest.Chunks)-1 && len(chunk.Data) != historyChunkBytes) {
				return nil, nil, fmt.Errorf("invalid access history chunk size")
			}
			raw = append(raw, chunk.Data...)
		}
		checksum := sha256.Sum256(raw)
		if len(raw) != manifest.Size || hex.EncodeToString(checksum[:]) != manifest.SHA256 {
			return nil, nil, fmt.Errorf("access history integrity mismatch")
		}
	}
	var event api.AccessChangeResource
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil, nil, fmt.Errorf("decode access history: %w", err)
	}
	if event.ID == nil || event.Properties == nil || event.Properties.Generation == nil ||
		*event.ID != historyPrefix+strconv.FormatInt(*event.Properties.Generation, 10) {
		return nil, nil, fmt.Errorf("invalid access history identity")
	}
	return &event, record, nil
}

type storedPolicy struct {
	policy Policy
}

// PolicyStore persists private immutable objects, not mutable public resource
// records. Only the ETag-protected anchor publishes a policy. It has no startup,
// HTTP, identity resolution, enforcement or background-task wiring.
type PolicyStore struct {
	db       database.Client
	catalog  *Catalog
	clock    clock.PassiveClock
	settings SettingsChangeGuard
}

// NewPolicyStore requires an already initialized anchor. Missing initialization
// is an explicit error on reads/writes, never an automatic bootstrap bypass.
func NewPolicyStore(db database.Client, catalog *Catalog, clk clock.PassiveClock, settings SettingsChangeGuard) (*PolicyStore, error) {
	if db == nil || catalog == nil {
		return nil, fmt.Errorf("database and trusted catalog are required")
	}
	if clk == nil {
		clk = clock.RealClock{}
	}
	return &PolicyStore{db: db, catalog: catalog, clock: clk, settings: settings}, nil
}

// InitializationVerifier must verify the trusted startup caller and complete
// initial policy, including bootstrap identities and mode prerequisites. No
// production implementation is supplied in this layer.
type InitializationVerifier interface {
	VerifyPolicyInitialization(context.Context, Policy) error
}

// PolicyInitializer is deliberately separate from the ordinary write API.
// Its verifier is a required trusted startup dependency, not a request flag.
type PolicyInitializer struct {
	store    *PolicyStore
	verifier InitializationVerifier
}

// NewPolicyInitializer constructs the future startup integration boundary.
func NewPolicyInitializer(store *PolicyStore, verifier InitializationVerifier) (*PolicyInitializer, error) {
	if store == nil || verifier == nil {
		return nil, fmt.Errorf("store and trusted initialization verifier are required")
	}
	return &PolicyInitializer{store: store, verifier: verifier}, nil
}

// Initialize atomically publishes one complete generation-zero policy. A second
// initializer always conflicts, even if it proposes identical policy. Prepared
// objects from a losing initializer are unreachable and are not garbage-collected.
func (i *PolicyInitializer) Initialize(ctx context.Context, initial Policy) error {
	policy, err := clonePolicyValue(initial)
	if err != nil {
		return err
	}
	if policy.Generation != 0 {
		return fmt.Errorf("initial policy must have generation zero")
	}
	if err := validateStoredPolicy(policy); err != nil {
		return err
	}
	for _, definition := range policy.CustomRoles {
		if _, err := compileRole(i.store.catalog, definition, false); err != nil {
			return err
		}
	}
	if _, err := NewPolicySnapshot(i.store.catalog, policy); err != nil {
		return err
	}
	// Give the trusted verifier its own copy so it cannot mutate the policy
	// between validation and publication.
	verification, err := clonePolicyValue(policy)
	if err != nil {
		return err
	}
	if err := i.verifier.VerifyPolicyInitialization(ctx, verification); err != nil {
		return fmt.Errorf("verify policy initialization: %w", err)
	}
	intent, err := uuid.NewRandom()
	if err != nil {
		return err
	}
	root, err := i.store.prepare(ctx, intent.String(), policy)
	if err != nil {
		return err
	}
	return i.store.save(ctx, accessVersionID, accessVersion{Root: root}, database.WithCreateOnly())
}

func validateStoredPolicy(policy Policy) error {
	if len(policy.CustomRoles) > 1000 || len(policy.Assignments) > 10000 {
		return fmt.Errorf("PolicyCapacityExceeded: maximum 1000 custom roles and 10000 assignments")
	}
	if policy.Settings.ID == nil || !strings.EqualFold(*policy.Settings.ID, settingsID) {
		return fmt.Errorf("settings ID must be %s", settingsID)
	}
	if policy.Generation < 0 || policy.Generation > maxGeneration ||
		policy.Settings.Properties == nil || policy.Settings.Properties.Generation == nil ||
		*policy.Settings.Properties.Generation != policy.Generation {
		return fmt.Errorf("persisted policy and settings generations must match")
	}
	tuples := map[string]bool{}
	for _, assignment := range policy.Assignments {
		if assignment.ID == nil || assignment.Properties == nil || assignment.Properties.RoleDefinitionID == nil {
			return fmt.Errorf("assignment ID, properties and role definition are required")
		}
		scope, err := extensionScope(*assignment.ID, "roleAssignments")
		if err != nil {
			return err
		}
		p := assignment.Properties
		principal, err := key(p.Principal)
		if err != nil {
			return err
		}
		if p.Scope == nil || !strings.EqualFold(*p.Scope, scope) || p.CreatedBy == nil || *p.CreatedBy == "" || p.ManagedBy == nil {
			return fmt.Errorf("assignment must have persisted server-derived scope, creator and management")
		}
		if err := validateManagement(*p.ManagedBy, true); err != nil {
			return err
		}
		// JSON avoids ambiguous delimiter concatenation for arbitrary subjects.
		tuple, err := json.Marshal([]string{scope, strings.ToLower(*p.RoleDefinitionID), string(principal.typ), principal.issuer, principal.subject})
		if err != nil {
			return err
		}
		if tuples[string(tuple)] {
			return fmt.Errorf("RoleAssignmentImmutable: duplicate assignment tuple")
		}
		tuples[string(tuple)] = true
	}
	return nil
}

func (s *PolicyStore) save(ctx context.Context, id string, value any, options ...database.SaveOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(raw) > maxPolicyBytes {
		return fmt.Errorf("PolicyCapacityExceeded: object %s exceeds %d encoded bytes", id, maxPolicyBytes)
	}
	obj := &database.Object{Metadata: database.Metadata{ID: id}, Data: json.RawMessage(raw)}
	if err := s.db.Save(ctx, obj, options...); err != nil {
		return fmt.Errorf("save %s: %w", id, err)
	}
	return nil
}

func (s *PolicyStore) read(ctx context.Context, id string, out any) (*database.Object, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	obj, err := s.db.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", id, err)
	}
	raw, err := json.Marshal(obj.Data)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxPolicyBytes {
		return nil, fmt.Errorf("PolicyCapacityExceeded: stored object %s exceeds byte bound", id)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, fmt.Errorf("decode %s: %w", id, err)
	}
	return obj, nil
}

func (s *PolicyStore) immutable(ctx context.Context, id string, value any) error {
	err := s.save(ctx, id, value, database.WithCreateOnly())
	var conflict *database.ErrConcurrency
	if !errors.As(err, &conflict) {
		return err
	}
	var existing json.RawMessage
	if _, err := s.read(ctx, id, &existing); err != nil {
		return err
	}
	expected, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var a, b any
	if err := json.Unmarshal(existing, &a); err != nil {
		return err
	}
	if err := json.Unmarshal(expected, &b); err != nil {
		return err
	}
	actual, err := json.Marshal(a)
	if err != nil {
		return err
	}
	canonical, err := json.Marshal(b)
	if err != nil {
		return err
	}
	if !bytes.Equal(actual, canonical) {
		return fmt.Errorf("immutable policy object collision at %s", id)
	}
	return nil
}

func (s *PolicyStore) prepare(ctx context.Context, intent string, policy Policy) (string, error) {
	if err := validateStoredPolicy(policy); err != nil {
		return "", err
	}
	type entry struct {
		id, kind string
		value    any
	}
	entries := []entry{{settingsID, "settings", policy.Settings}}
	for _, r := range policy.CustomRoles {
		entries = append(entries, entry{strings.ToLower(*r.ID), "role", r})
	}
	for _, a := range policy.Assignments {
		entries = append(entries, entry{strings.ToLower(*a.ID), "assignment", a})
	}
	slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.id, b.id) })
	root := manifestRoot{Generation: policy.Generation}
	page := manifestPage{}
	payload := policyPayload{Entries: map[string]json.RawMessage{}}
	payloadBytes := len(`{"entries":{}}`)
	flush := func() error {
		if len(root.Pages) == manifestFanout {
			return &policyMutationError{fmt.Errorf("PolicyCapacityExceeded: manifest root fanout")}
		}
		index := strconv.Itoa(len(root.Pages))
		page.Object = policyObjectID + intent + "-object-" + index
		if err := s.immutable(ctx, page.Object, payload); err != nil {
			return err
		}
		id := policyObjectID + intent + "-page-" + index
		if err := s.immutable(ctx, id, page); err != nil {
			return err
		}
		root.Pages = append(root.Pages, id)
		page = manifestPage{}
		payload = policyPayload{Entries: map[string]json.RawMessage{}}
		payloadBytes = len(`{"entries":{}}`)
		return nil
	}
	for _, e := range entries {
		raw, err := json.Marshal(e.value)
		if err != nil {
			return "", err
		}
		key, err := json.Marshal(e.id)
		if err != nil {
			return "", err
		}
		entryBytes := len(key) + 1 + len(raw)
		if len(`{"entries":{}}`)+entryBytes > maxPolicyBytes {
			return "", &policyMutationError{fmt.Errorf("PolicyCapacityExceeded: policy entry %s exceeds payload byte bound", e.id)}
		}
		if len(page.Entries) > 0 && (len(page.Entries) == manifestFanout || payloadBytes+1+entryBytes > maxPolicyBytes) {
			if err := flush(); err != nil {
				return "", err
			}
		}
		if len(page.Entries) > 0 {
			payloadBytes++
		}
		payload.Entries[e.id] = raw
		payloadBytes += entryBytes
		page.Entries = append(page.Entries, policyReference{ID: e.id, Kind: e.kind})
	}
	if err := flush(); err != nil {
		return "", err
	}
	id := policyObjectID + intent + "-root"
	if err := s.immutable(ctx, id, root); err != nil {
		return "", err
	}
	return id, nil
}

func (s *PolicyStore) loadRoot(ctx context.Context, id string, generation int64) (*storedPolicy, error) {
	var root manifestRoot
	if !strings.HasPrefix(id, policyObjectID) {
		return nil, fmt.Errorf("invalid private policy root")
	}
	if _, err := s.read(ctx, id, &root); err != nil {
		return nil, err
	}
	if root.Generation != generation || len(root.Pages) == 0 || len(root.Pages) > manifestFanout {
		return nil, fmt.Errorf("invalid policy manifest generation or fanout")
	}
	result := &storedPolicy{policy: Policy{Generation: generation}}
	seen := map[string]bool{}
	settings := 0
	for _, pageID := range root.Pages {
		if !strings.HasPrefix(pageID, policyObjectID) {
			return nil, fmt.Errorf("invalid private policy page")
		}
		var page manifestPage
		if _, err := s.read(ctx, pageID, &page); err != nil {
			return nil, err
		}
		if len(page.Entries) == 0 || len(page.Entries) > manifestFanout {
			return nil, fmt.Errorf("invalid policy page fanout")
		}
		if !strings.HasPrefix(page.Object, policyObjectID) {
			return nil, fmt.Errorf("invalid private policy payload")
		}
		var payload policyPayload
		if _, err := s.read(ctx, page.Object, &payload); err != nil {
			return nil, err
		}
		if len(payload.Entries) != len(page.Entries) {
			return nil, fmt.Errorf("policy payload and manifest entry count mismatch")
		}
		for _, ref := range page.Entries {
			if seen[ref.ID] {
				return nil, fmt.Errorf("duplicate or invalid policy reference")
			}
			raw, exists := payload.Entries[ref.ID]
			if !exists {
				return nil, fmt.Errorf("policy reference missing from payload")
			}
			switch ref.Kind {
			case "role":
				var value api.RoleDefinitionResource
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, err
				}
				if value.ID == nil || strings.ToLower(*value.ID) != ref.ID {
					return nil, fmt.Errorf("role manifest ID mismatch")
				}
				result.policy.CustomRoles = append(result.policy.CustomRoles, value)
			case "assignment":
				var value api.RoleAssignmentResource
				if err := json.Unmarshal(raw, &value); err != nil {
					return nil, err
				}
				if value.ID == nil || strings.ToLower(*value.ID) != ref.ID {
					return nil, fmt.Errorf("assignment manifest ID mismatch")
				}
				result.policy.Assignments = append(result.policy.Assignments, value)
			case "settings":
				if ref.ID != settingsID {
					return nil, fmt.Errorf("settings manifest ID mismatch")
				}
				if err := json.Unmarshal(raw, &result.policy.Settings); err != nil {
					return nil, err
				}
				settings++
			default:
				return nil, fmt.Errorf("unknown policy entry kind %s", ref.Kind)
			}
			seen[ref.ID] = true
		}
	}
	if settings != 1 {
		return nil, fmt.Errorf("policy must have exactly one settings record")
	}
	if err := validateStoredPolicy(result.policy); err != nil {
		return nil, err
	}
	if _, err := NewPolicySnapshot(s.catalog, result.policy); err != nil {
		return nil, err
	}
	return result, nil
}

type policyRetryBudget struct{ conflicts int }

func (b *policyRetryBudget) retry() error {
	b.conflicts++
	if b.conflicts > policyRetries {
		return ErrPolicyConflict
	}
	return nil
}

func (s *PolicyStore) stableVersion(ctx context.Context, budget *policyRetryBudget) (accessVersion, *database.Object, error) {
	for attempt := 0; attempt <= policyRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return accessVersion{}, nil, err
		}
		var version accessVersion
		obj, err := s.read(ctx, accessVersionID, &version)
		if err != nil {
			return accessVersion{}, nil, fmt.Errorf("%w: policy initialization or version read failed: %w", ErrAuthorizationUnavailable, err)
		}
		if version.Generation < 0 || version.Generation > maxGeneration || obj.ETag == "" || version.Root == "" {
			return accessVersion{}, nil, fmt.Errorf("%w: invalid access version", ErrAuthorizationUnavailable)
		}
		if version.Pending == nil {
			return version, obj, nil
		}
		if err := s.finish(ctx, version, obj.ETag); err != nil {
			var conflict *database.ErrConcurrency
			if !errors.As(err, &conflict) {
				return accessVersion{}, nil, err
			}
			if err := budget.retry(); err != nil {
				return accessVersion{}, nil, err
			}
		}
	}
	return accessVersion{}, nil, ErrPolicyConflict
}

func (s *PolicyStore) finish(ctx context.Context, version accessVersion, etag string) error {
	pending := version.Pending
	if pending == nil || pending.Generation != version.Generation+1 || pending.Generation > maxGeneration ||
		!strings.HasPrefix(pending.Root, policyObjectID+pending.Intent+"-") ||
		pending.Event != policyObjectID+pending.Intent+"-event" {
		return fmt.Errorf("invalid pending policy intent")
	}
	if _, err := uuid.Parse(pending.Intent); err != nil {
		return fmt.Errorf("invalid pending policy intent UUID: %w", err)
	}
	if _, err := s.loadRoot(ctx, pending.Root, pending.Generation); err != nil {
		return err
	}
	event, record, err := s.readHistory(ctx, pending.Event)
	if err != nil {
		return err
	}
	historyID := historyPrefix + strconv.FormatInt(pending.Generation, 10)
	if event.Properties == nil || event.Properties.Generation == nil || *event.Properties.Generation != pending.Generation ||
		event.ID == nil || *event.ID != historyID {
		return fmt.Errorf("invalid pending access history")
	}
	if err := s.immutable(ctx, historyID, record); err != nil {
		return err
	}
	return s.save(ctx, accessVersionID, accessVersion{Generation: pending.Generation, Root: pending.Root}, database.WithETag(etag))
}

// LoadPolicy recovers durable pending intent before returning committed data.
// The unchanged-generation fast path reads only the shared anchor.
func (s *PolicyStore) LoadPolicy(ctx context.Context, knownGeneration int64) (*PolicySnapshot, error) {
	return s.loadPolicy(ctx, knownGeneration, &policyRetryBudget{})
}

func (s *PolicyStore) loadPolicy(ctx context.Context, knownGeneration int64, budget *policyRetryBudget) (*PolicySnapshot, error) {
	version, _, err := s.stableVersion(ctx, budget)
	if err != nil {
		return nil, err
	}
	if knownGeneration == version.Generation {
		return nil, nil
	}
	stored, err := s.loadRoot(ctx, version.Root, version.Generation)
	if err != nil {
		return nil, err
	}
	return NewPolicySnapshot(s.catalog, stored.policy)
}

// Mutate re-reads complete committed policy and reruns all checks on each
// conflict. It never retries arbitrary database errors as successful writes.
// A returned error after claiming intent can mean a committed change pending
// recovery; callers must inspect policy/history rather than assume rollback.
func (s *PolicyStore) Mutate(ctx context.Context, actor PolicyActor, mutation PolicyMutation) (*PolicySnapshot, error) {
	actor, err := clonePolicyValue(actor)
	if err != nil {
		return nil, err
	}
	mutation, err = clonePolicyValue(mutation)
	if err != nil {
		return nil, err
	}
	budget := &policyRetryBudget{}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		version, obj, err := s.stableVersion(ctx, budget)
		if err != nil {
			return nil, err
		}
		stored, err := s.loadRoot(ctx, version.Root, version.Generation)
		if err != nil {
			return nil, err
		}
		current, err := NewPolicySnapshot(s.catalog, stored.policy)
		if err != nil {
			return nil, err
		}
		// Validation may populate server-owned properties; isolate each retry.
		input, err := clonePolicyValue(mutation)
		if err != nil {
			return nil, err
		}
		next, change, highImpact, err := applyPolicyMutation(ctx, current, actor, input, s.settings)
		if err != nil {
			return nil, &policyMutationError{err}
		}
		if change == nil {
			return current, nil
		}
		if version.Generation == maxGeneration {
			return nil, &policyMutationError{fmt.Errorf("PolicyCapacityExceeded: generation space exhausted")}
		}
		next.Generation = version.Generation + 1
		next.Settings.Properties.Generation = to.Ptr(next.Generation)
		intent, err := uuid.NewRandom()
		if err != nil {
			return nil, err
		}
		root, err := s.prepare(ctx, intent.String(), next)
		if err != nil {
			return nil, err
		}
		eventID := policyObjectID + intent.String() + "-event"
		event := api.AccessChangeResource{
			ID: to.Ptr(historyPrefix + strconv.FormatInt(next.Generation, 10)),
			Properties: &api.AccessChangeProperties{
				Generation: to.Ptr(next.Generation), HighImpact: to.Ptr(highImpact),
				Mode: next.Settings.Properties.Mode, PolicyChange: change,
				Principal: &actor.Identity.Principal, Recovery: to.Ptr(actor.Recovery),
				RequestID: to.Ptr(actor.RequestID), CorrelationID: to.Ptr(actor.CorrelationID),
				Time: to.Ptr(s.clock.Now().UTC()),
			},
		}
		if err := s.prepareHistory(ctx, eventID, event); err != nil {
			return nil, err
		}
		version.Pending = &pendingPolicy{Generation: next.Generation, Intent: intent.String(), Root: root, Event: eventID}
		if err := s.save(ctx, accessVersionID, version, database.WithETag(obj.ETag)); err != nil {
			var conflict *database.ErrConcurrency
			if errors.As(err, &conflict) {
				if err := budget.retry(); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		// Do not use the pre-save ETag: Save computes the content ETag itself.
		// Reading again also lets another instance finish on our behalf.
		if _, err := s.loadPolicy(ctx, -1, budget); err != nil {
			return nil, err
		}
		// A later writer may already have published another generation. Return
		// this mutation's committed value, not that writer's replacement.
		return NewPolicySnapshot(s.catalog, next)
	}
}

// MutateAndRefresh is the common local-write integration path for a cache.
// A successful mutation is followed immediately by a version check, including
// idempotent writes. Refresh failures are returned, not hidden as write success.
func (s *PolicyStore) MutateAndRefresh(ctx context.Context, cache *PolicyCache, actor PolicyActor, mutation PolicyMutation) (*PolicySnapshot, error) {
	if cache == nil || cache.source != s {
		return nil, fmt.Errorf("local refresh cache must use this policy store")
	}
	result, err := s.Mutate(ctx, actor, mutation)
	if err != nil {
		return nil, err
	}
	if err := cache.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("policy committed but local refresh failed: %w: %w", ErrAuthorizationUnavailable, err)
	}
	return result, nil
}

// AccessChange reads immutable committed history by generation, recovering
// pending intent first. Prepared and uncommitted history is never returned.
func (s *PolicyStore) AccessChange(ctx context.Context, generation int64) (*api.AccessChangeResource, error) {
	version, _, err := s.stableVersion(ctx, &policyRetryBudget{})
	if err != nil {
		return nil, err
	}
	if generation <= 0 || generation > version.Generation {
		return nil, &database.ErrNotFound{ID: historyPrefix + strconv.FormatInt(generation, 10)}
	}
	event, _, err := s.readHistory(ctx, historyPrefix+strconv.FormatInt(generation, 10))
	if err != nil {
		return nil, err
	}
	if *event.Properties.Generation != generation {
		return nil, fmt.Errorf("access history generation mismatch")
	}
	return event, nil
}

var _ PolicySource = (*PolicyStore)(nil)
