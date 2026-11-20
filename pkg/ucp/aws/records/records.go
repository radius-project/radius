/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package records reads and writes UCP's record of each AWS resource it deploys (datamodel.AWSResource). A record is
// stored in UCP's data store under the resource ID that ends in the alias.
package records

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	awsoperations "github.com/radius-project/radius/pkg/aws/operations"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_aws "github.com/radius-project/radius/pkg/ucp/resources/aws"
)

// OperationType is the async operation type of AWS resource operations. The frontend queues PUT and DELETE operations
// under it, and the backend controller that waits for AWS is registered under it.
const OperationType = "AWSRESOURCE"

// NativeOperationType is the async operation type of AWS resource operations a native handler serves. The type
// travels with the queued operation, so a worker that finds the record gone still knows Cloud Control has no request
// for it.
const NativeOperationType = "AWSRESOURCENATIVE"

// Operation names, as saved in PendingOperation.
const (
	OperationCreate = "CREATE"
	OperationUpdate = "UPDATE"
	OperationDelete = "DELETE"
)

// HandlerNative is the record's Handler when a native handler serves its operation. An empty Handler means Cloud
// Control.
const HandlerNative = "native"

// StaleAfter is how long an operation in progress can go without a poll before UCP treats it as stale. The worker
// saves the record at least every PollInterval while it polls AWS, so an operation a worker is polling never looks
// stale, however long AWS takes.
const StaleAfter = 10 * time.Minute

// PollInterval is how often the worker saves the record while it polls AWS. It is well under StaleAfter.
const PollInterval = 2 * time.Minute

// Now returns the current time. Tests replace it.
var Now = time.Now

// Store reads and writes records in UCP's data store.
type Store struct {
	db database.Client
}

// NewStore creates a Store.
func NewStore(db database.Client) *Store {
	return &Store{db: db}
}

// Get returns the record stored under id and its ETag, or nil when there is none. Any other read failure is an error:
// a failed read is never treated as "no record", because UCP would then create a duplicate.
func (s *Store) Get(ctx context.Context, id resources.ID) (*datamodel.AWSResource, string, error) {
	obj, err := s.db.Get(ctx, id.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", fmt.Errorf("failed to read the record of %s: %w", id.String(), err)
	}
	rec := &datamodel.AWSResource{}
	if err := convert(obj.Data, rec); err != nil {
		return nil, "", fmt.Errorf("failed to read the record of %s: %w", id.String(), err)
	}
	return rec, obj.ETag, nil
}

// Save writes a record and returns its new ETag. etag is the ETag the record was read with, and the write fails with
// database.ErrConcurrency when the record changed since. An empty etag means the record is new, and the write fails
// with database.ErrConcurrency when another request saved one first, so two first deploys of an alias never both
// create a resource. Save sets systemData.lastModifiedAt, which tells a running operation from a stale one.
func (s *Store) Save(ctx context.Context, rec *datamodel.AWSResource, etag string) (string, error) {
	rec.SystemData.LastModifiedAt = Now().UTC().Format(time.RFC3339Nano)
	data := map[string]any{}
	if err := convert(rec, &data); err != nil {
		return "", err
	}
	obj := &database.Object{Metadata: database.Metadata{ID: rec.ID}, Data: data}
	opt := database.WithCreateOnly()
	if etag != "" {
		opt = database.WithETag(etag)
	}
	if err := s.db.Save(ctx, obj, opt); err != nil {
		return "", err
	}
	return obj.ETag, nil
}

// DeleteIfUnchanged removes the record stored under id when its ETag is still etag. It fails with
// database.ErrConcurrency when the record changed or was removed since it was read.
func (s *Store) DeleteIfUnchanged(ctx context.Context, id resources.ID, etag string) error {
	err := s.db.Delete(ctx, id.String(), database.WithETag(etag))
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil
	}
	return err
}

// Touch saves the record unchanged, so its lastModifiedAt moves forward. The worker calls it while it polls AWS, so
// the operation does not look stale. A record that changed or was removed meanwhile is left alone.
func (s *Store) Touch(ctx context.Context, id resources.ID) error {
	rec, etag, err := s.Get(ctx, id)
	if err != nil || rec == nil {
		return err
	}
	_, err = s.Save(ctx, rec, etag)
	if errors.Is(err, &database.ErrConcurrency{}) {
		return nil
	}
	return err
}

// FindByIdentifier returns the resource ID of another alias whose record has the AWS identifier, for the same type
// as id, in the type's identity scope, or "" when there is none. UCP calls it before it takes over a resource, or
// creates one whose identifier the properties give, so two aliases never manage the same AWS resource.
//
// With schemas.IdentityScopeAccount, the search covers every region of id's account, so an alias in another region
// that sets the same account-wide setting is found. Otherwise it covers id's account and region.
//
// The search and the save that follows it are separate steps, so two aliases that take over the same identifier at
// the same moment can both succeed.
func (s *Store) FindByIdentifier(ctx context.Context, id resources.ID, identifier string, scope schemas.IdentityScope) (string, error) {
	if identifier == "" {
		return "", nil
	}
	query := database.Query{
		RootScope:    id.RootScope(),
		ResourceType: id.Type(),
		Filters:      []database.QueryFilter{{Field: "properties.awsIdentifier", Value: identifier}},
	}
	if scope == schemas.IdentityScopeAccount {
		query.RootScope, query.ScopeRecursive = accountScope(id), true
	}
	token := ""
	for {
		opts := []database.QueryOptions{}
		if token != "" {
			opts = append(opts, database.WithPaginationToken(token))
		}
		result, err := s.db.Query(ctx, query, opts...)
		if err != nil {
			return "", fmt.Errorf("failed to search the records for the AWS resource %q: %w", identifier, err)
		}
		for _, item := range result.Items {
			if !strings.EqualFold(item.ID, id.String()) {
				return item.ID, nil
			}
		}
		if result.PaginationToken == "" {
			return "", nil
		}
		token = result.PaginationToken
	}
}

// Others returns the records of other aliases of id's type in the identity scope, as FindByIdentifier searches them.
// UCP reads them to find the members of a parent that another alias may hold (see native.Members). Like
// FindByIdentifier, it doesn't stop another alias from saving a record right after.
func (s *Store) Others(ctx context.Context, id resources.ID, scope schemas.IdentityScope) ([]*datamodel.AWSResource, error) {
	query := database.Query{RootScope: id.RootScope(), ResourceType: id.Type()}
	if scope == schemas.IdentityScopeAccount {
		query.RootScope, query.ScopeRecursive = accountScope(id), true
	}
	out := []*datamodel.AWSResource{}
	token := ""
	for {
		opts := []database.QueryOptions{}
		if token != "" {
			opts = append(opts, database.WithPaginationToken(token))
		}
		result, err := s.db.Query(ctx, query, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to read the records of %s: %w", id.Type(), err)
		}
		for _, item := range result.Items {
			if strings.EqualFold(item.ID, id.String()) {
				continue
			}
			rec := &datamodel.AWSResource{}
			if err := convert(item.Data, rec); err != nil {
				return nil, fmt.Errorf("failed to read the record of %s: %w", item.ID, err)
			}
			out = append(out, rec)
		}
		if result.PaginationToken == "" {
			return out, nil
		}
		token = result.PaginationToken
	}
}

// accountScope returns the account scope of an AWS resource ID, such as /planes/aws/aws/accounts/1234, or its root
// scope when it has no account.
func accountScope(id resources.ID) string {
	scopes := id.ScopeSegments()
	for i, scope := range scopes {
		if strings.EqualFold(scope.Type, resources_aws.ScopeAccounts) {
			return resources.MakeUCPID(scopes[:i+1], nil, nil)
		}
	}
	return id.RootScope()
}

// NewRecord returns an empty record for the resource id.
func NewRecord(id resources.ID, owner string) *datamodel.AWSResource {
	rec := &datamodel.AWSResource{}
	rec.ID = id.String()
	rec.Name = id.Name()
	rec.Type = id.Type()
	rec.Properties.Owner = owner
	return rec
}

// LastModified returns when the record was last saved, or the zero time when it is unknown.
func LastModified(rec *datamodel.AWSResource) time.Time {
	t, err := time.Parse(time.RFC3339Nano, rec.SystemData.LastModifiedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// IsRunning reports whether the record shows an operation in progress. A running operation can still be stale, see
// IsStale.
func IsRunning(rec *datamodel.AWSResource) bool {
	switch rec.ProvisioningState() {
	case v1.ProvisioningStateProvisioning, v1.ProvisioningStateUpdating, v1.ProvisioningStateDeleting:
		return true
	default:
		return false
	}
}

// IsStale reports whether nothing has saved the record for StaleAfter. The worker saves the record while it polls
// AWS, so a running operation that is stale has no worker: UCP stopped before it queued the operation, or the worker
// stopped polling it.
func IsStale(rec *datamodel.AWSResource) bool {
	last := LastModified(rec)
	return last.IsZero() || Now().Sub(last) > StaleAfter
}

// IsFirstCreate reports whether the record's first create has not finished.
func IsFirstCreate(rec *datamodel.AWSResource) bool {
	return rec.Properties.ClientToken != ""
}

// HasApplied reports whether the record holds the properties of a successful deploy. A record saved before the first
// create, or when UCP took over an existing resource, has none, so nothing is cleared on its next deploy.
func HasApplied(rec *datamodel.AWSResource) bool {
	return rec.Properties.LastApplied != nil
}

// PendingOperation returns the operation the record shows in progress: PendingOperation when set, otherwise DELETE for
// a record being deleted, CREATE while it holds a client token, and UPDATE.
func PendingOperation(rec *datamodel.AWSResource) string {
	switch {
	case rec.Properties.PendingOperation != "":
		return rec.Properties.PendingOperation
	case rec.ProvisioningState() == v1.ProvisioningStateDeleting:
		return OperationDelete
	case IsFirstCreate(rec):
		return OperationCreate
	default:
		return OperationUpdate
	}
}

// StateOf returns the provisioning state of a record whose operation op is in progress.
func StateOf(op string) v1.ProvisioningState {
	switch op {
	case OperationCreate:
		return v1.ProvisioningStateProvisioning
	case OperationDelete:
		return v1.ProvisioningStateDeleting
	default:
		return v1.ProvisioningStateUpdating
	}
}

// ClearPending drops the values of the operation in progress, so the previous last-applied properties and
// fingerprints stay.
func ClearPending(rec *datamodel.AWSResource) {
	rec.Properties.PendingLastApplied = nil
	rec.Properties.PendingFingerprints = nil
	rec.Properties.PendingOperation = ""
}

// KeepPending treats the values of an interrupted update as applied: the update may have reached AWS, so Radius owns
// the pending properties, and a later deploy that omits one clears it. The pending values are then dropped.
//
// A path whose pending value differs from the last-applied one is added to UnresolvedPaths, because AWS may hold
// either value or a mix. That matters for a list, which the pending value replaces whole: a field inside a list item
// that the update was removing would otherwise no longer be owned, though AWS may still have it.
//
// Whether AWS has a write-only value the update sent cannot be known, so its fingerprint is not kept as applied, and
// the previous one is dropped too. The next deploy sends that value again. Fingerprints that the update did not change
// stay.
func KeepPending(rec *datamodel.AWSResource) {
	if rec.Properties.PendingLastApplied != nil {
		if rec.Properties.LastApplied == nil {
			rec.Properties.LastApplied = map[string]any{}
		}
		markUnresolved(rec)
		merge(rec.Properties.LastApplied, rec.Properties.PendingLastApplied)
	}
	previous, pending := rec.Properties.Fingerprints, rec.Properties.PendingFingerprints
	if previous != nil && pending != nil && previous.KeyID == pending.KeyID {
		for path, digest := range pending.Digests {
			if previous.Digests[path] != digest {
				delete(previous.Digests, path)
			}
		}
	}
	ClearPending(rec)
}

// markUnresolved adds to UnresolvedPaths every path whose pending value differs from the last-applied one, from
// either side, because AWS may hold either value or a mix: the operation in progress failed partway, or UCP can't tell
// how it ended. A path only one side has counts too, such as a list the pending values change into an object.
func markUnresolved(rec *datamodel.AWSResource) {
	paths := rec.Properties.UnresolvedPaths
	applied := awsoperations.FlattenProperties(rec.Properties.LastApplied)
	pending := awsoperations.FlattenProperties(rec.Properties.PendingLastApplied)
	for _, values := range []map[string]any{applied, pending} {
		for path := range values {
			if !reflect.DeepEqual(applied[path], pending[path]) {
				paths = append(paths, path)
			}
		}
	}
	slices.Sort(paths)
	rec.Properties.UnresolvedPaths = slices.Compact(paths)
}

// merge copies src into dst. When both hold an object under the same key, the objects are merged.
func merge(dst map[string]any, src map[string]any) {
	for k, v := range src {
		srcMap, srcIsMap := v.(map[string]any)
		dstMap, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			merge(dstMap, srcMap)
			continue
		}
		dst[k] = v
	}
}

// ApplyPending finishes a successful create or update: the pending last-applied properties and fingerprints become
// the last-applied ones, and the operation, the tokens, and the pending values are cleared. Clearing the client token
// marks the first create as finished. The request token moves to LastRequestToken, so a redelivered operation reports
// success.
func ApplyPending(rec *datamodel.AWSResource) {
	applyPendingValues(rec)
	FinishCreate(rec)
	ClearUpdateToken(rec)
	finish(rec, nil)
	rec.SetProvisioningState(v1.ProvisioningStateSucceeded)
}

// applyPendingValues makes the pending last-applied properties and fingerprints the last-applied ones, and drops the
// pending values. AWS applied all of them, so no path is unresolved any more.
func applyPendingValues(rec *datamodel.AWSResource) {
	if rec.Properties.PendingLastApplied != nil {
		rec.Properties.LastApplied = rec.Properties.PendingLastApplied
		rec.Properties.Fingerprints = rec.Properties.PendingFingerprints
		rec.Properties.UnresolvedPaths = nil
	}
	ClearPending(rec)
}

// Settle sets the record's values after a failed operation by how much of it AWS applied: OutcomeRejected drops the
// pending values (ClearPending), OutcomeApplied makes them last-applied, and OutcomePartial and OutcomeUnknown keep
// them as possibly applied (KeepPending). It leaves the operation's state to the caller.
func Settle(rec *datamodel.AWSResource, outcome native.Outcome) {
	switch outcome {
	case native.OutcomeRejected:
		ClearPending(rec)
	case native.OutcomeApplied:
		applyPendingValues(rec)
	default:
		KeepPending(rec)
	}
}

// Fail finishes a failed update or delete: the pending values are dropped, so the previous last-applied properties
// and fingerprints stay. The request token moves to LastRequestToken with failure, so a redelivered operation reports
// the same error. The update token is cleared, because AWS finished the update, so the next update is a new one (see
// FailUnfinished).
func Fail(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	ClearPending(rec)
	ClearUpdateToken(rec)
	finish(rec, failure)
	rec.SetProvisioningState(v1.ProvisioningStateFailed)
}

// FailUnfinished finishes an update UCP stopped waiting for before AWS finished it, such as at the timeout, as Fail
// does, but keeps the update token and the handler's state for it. AWS accepted the update and may still run it, so a
// deploy of the same values sends it again with its token, and AWS answers with the update it has, instead of taking
// it as a new one.
func FailUnfinished(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	ClearPending(rec)
	finish(rec, failure)
	rec.SetProvisioningState(v1.ProvisioningStateFailed)
}

func finish(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	ClearStart(rec)
	rec.Properties.LastRequestToken = rec.Properties.RequestToken
	rec.Properties.LastRequestError = failure
	rec.Properties.RequestToken = ""
}

// FailStart finishes an operation whose start call failed in the worker, after StartFailed set the record's values.
// Unlike Fail, it keeps the pending values StartFailed kept: a create AWS may have keeps the values the next request
// finds it with (see native.Recovery). The request token moves to LastRequestToken with failure.
func FailStart(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	finish(rec, failure)
	rec.SetProvisioningState(v1.ProvisioningStateFailed)
}

// FinishCreate marks the first create as finished: it clears the client token, and keeps it as the create token, so a
// native handler's cleanup can tell the marks of this create from another alias's (see native.Cleaner). The new
// resource isn't cleaned up yet, even when one this record had before was. It changes nothing for a record whose
// first create already finished.
func FinishCreate(rec *datamodel.AWSResource) {
	if rec.Properties.ClientToken != "" {
		rec.Properties.CreateToken = rec.Properties.ClientToken
		rec.Properties.CleanedUpAt = nil
	}
	rec.Properties.ClientToken = ""
}

// ClearUpdateToken clears the update token and the handler's state for it, so the next update is a new one.
func ClearUpdateToken(rec *datamodel.AWSResource) {
	rec.Properties.UpdateToken = ""
	rec.Properties.UpdateState = ""
	rec.Properties.UpdateIntent = ""
}

// OutcomeOf returns how much of a start call that failed with err AWS applied: the outcome a native handler reports
// with a *native.Failure, OutcomeRejected for a client fault, such as an invalid property, for a resource of another
// product, which the handler refuses before it changes anything, and for a call AWS can't take yet
// (native.NotReady), and OutcomeUnknown otherwise, because a transport or server error does not show that AWS changed
// nothing.
func OutcomeOf(err error) native.Outcome {
	var failure *native.Failure
	if errors.As(err, &failure) {
		return failure.Outcome
	}
	var other *native.OtherProductError
	var notReady *native.NotReady
	if errors.As(err, &other) || errors.As(err, &notReady) || ucp_aws.IsClientFault(err) {
		return native.OutcomeRejected
	}
	return native.OutcomeUnknown
}

// Start states of a native operation whose first call waits for AWS to take it (see native.NotReady), saved in
// StartState.
const (
	// StartWaiting means nothing of the operation has reached AWS, so a new request can replace it.
	StartWaiting = "waiting"

	// StartSending means the worker may be sending the call, so AWS may have it.
	StartSending = "sending"
)

// IsWaiting reports whether the record's operation waits to send its first call and nothing of it has reached AWS.
func IsWaiting(rec *datamodel.AWSResource) bool {
	return rec.Properties.StartState == StartWaiting
}

// ClearStart drops what the record keeps for a first call that waits for AWS, once AWS took it or the operation
// ended.
func ClearStart(rec *datamodel.AWSResource) {
	rec.Properties.StartState = ""
	rec.Properties.StartReason = ""
	rec.Properties.StartPatch = nil
	rec.Properties.StartCurrent = nil
}

// StartFailed sets the record's values after the call that starts op failed with outcome, the same way whether UCP
// made the call at the request or in the worker after waiting. created is the identifier a create returned with its
// error. resend is true when the call sent again an operation that may already be in AWS: a create of a record that
// already had a client token, or an update with the token of an earlier attempt.
//
//   - create: the record keeps created. When AWS applied all of it and UCP has the identifier, the create is finished
//     and its values are last-applied. A rejected first create leaves nothing in AWS, so it returns true: the request
//     removes the record, and the worker clears the create from it (DropCreate). Otherwise the record keeps its client token, so the next request finds the resource.
//   - update: the values are set by outcome (Settle). A rejected first attempt clears the update token, so the next
//     update gets a new one. A rejected resend keeps it, because AWS may have taken an earlier attempt.
//   - delete: the pending values are dropped. When AWS applied all of it, it returns true: the caller removes the
//     record.
//
// It leaves the provisioning state and the request token to the caller.
func StartFailed(rec *datamodel.AWSResource, op string, outcome native.Outcome, created string, resend bool) bool {
	ClearStart(rec)
	switch op {
	case OperationCreate:
		if created != "" {
			rec.Properties.AWSIdentifier = created
		}
		if outcome == native.OutcomeRejected && !resend {
			return true
		}
		if outcome == native.OutcomeApplied && rec.Properties.AWSIdentifier != "" {
			Settle(rec, outcome)
			FinishCreate(rec)
		}
	case OperationUpdate:
		Settle(rec, outcome)
		if outcome == native.OutcomeRejected && !resend {
			ClearUpdateToken(rec)
		}
	case OperationDelete:
		ClearPending(rec)
		return outcome == native.OutcomeApplied
	}
	return false
}

// DropCreate clears a first create AWS never had from the record, so the record holds nothing but its owner, its
// handler, and its operation, like a new record: the next PUT creates a resource with a new client token or takes one
// over, a DELETE removes only the record, and no other alias is kept from the identifier. The record keeps nothing of a
// resource it held before, such as one deleted outside Radius, as a request that removes the record would. UCP's
// worker keeps such a record, rather than removing it, so a redelivered operation still reports the failure saved on
// it.
func DropCreate(rec *datamodel.AWSResource) {
	p := rec.Properties
	rec.Properties = datamodel.AWSResourceProperties{
		Owner:            p.Owner,
		Handler:          p.Handler,
		RequestToken:     p.RequestToken,
		LastRequestToken: p.LastRequestToken,
		LastRequestError: p.LastRequestError,
	}
}

// DropWaiting ends the record's waiting operation (see IsWaiting) as if AWS rejected its first call, because nothing
// of it reached AWS: a waiting first create returns true, and the caller removes the record or clears the create from
// it (DropCreate); an update drops its values
// and token; a delete drops its values and its delete token, which no call took, so the next delete is a new one. It
// leaves the provisioning state and the request token to the caller.
func DropWaiting(rec *datamodel.AWSResource) bool {
	if PendingOperation(rec) == OperationDelete {
		rec.Properties.DeleteToken = ""
	}
	return StartFailed(rec, PendingOperation(rec), native.OutcomeRejected, "", false)
}

// AliasOf returns the alias, the last segment, of a record's resource ID.
func AliasOf(resourceID string) string {
	if id, err := resources.ParseResource(resourceID); err == nil {
		return id.Name()
	}
	return resourceID
}

func convert(in any, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
