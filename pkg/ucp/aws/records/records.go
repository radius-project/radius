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
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// OperationType is the async operation type of AWS resource operations. The frontend queues PUT and DELETE operations
// under it, and the backend controller that waits for AWS is registered under it.
const OperationType = "AWSRESOURCE"

// Operation names, as saved in PendingOperation.
const (
	OperationCreate = "CREATE"
	OperationUpdate = "UPDATE"
	OperationDelete = "DELETE"
)

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

// Delete removes the record stored under id. A missing record is not an error.
func (s *Store) Delete(ctx context.Context, id resources.ID) error {
	err := s.db.Delete(ctx, id.String())
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil
	}
	return err
}

// DeleteIfUnchanged removes the record stored under id when its ETag is still etag. It fails with
// database.ErrConcurrency when the record changed since it was read. A missing record is not an error.
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

// FindByIdentifier returns the resource ID of another alias whose record has the AWS identifier, in the same account,
// region, and type as id, or "" when there is none. UCP calls it before it takes over a resource, or creates one
// whose identifier the properties give, so two aliases never manage the same AWS resource.
func (s *Store) FindByIdentifier(ctx context.Context, id resources.ID, identifier string) (string, error) {
	if identifier == "" {
		return "", nil
	}
	query := database.Query{
		RootScope:    id.RootScope(),
		ResourceType: id.Type(),
		Filters:      []database.QueryFilter{{Field: "properties.awsIdentifier", Value: identifier}},
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
// Whether AWS has a write-only value the update sent cannot be known, so its fingerprint is not kept as applied, and
// the previous one is dropped too. The next deploy sends that value again. Fingerprints that the update did not change
// stay.
func KeepPending(rec *datamodel.AWSResource) {
	if rec.Properties.PendingLastApplied != nil {
		if rec.Properties.LastApplied == nil {
			rec.Properties.LastApplied = map[string]any{}
		}
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
	if rec.Properties.PendingLastApplied != nil {
		rec.Properties.LastApplied = rec.Properties.PendingLastApplied
		rec.Properties.Fingerprints = rec.Properties.PendingFingerprints
	}
	ClearPending(rec)
	rec.Properties.ClientToken = ""
	finish(rec, nil)
	rec.SetProvisioningState(v1.ProvisioningStateSucceeded)
}

// Fail finishes a failed update or delete: the pending values are dropped, so the previous last-applied properties
// and fingerprints stay. The request token moves to LastRequestToken with failure, so a redelivered operation reports
// the same error.
func Fail(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	ClearPending(rec)
	finish(rec, failure)
	rec.SetProvisioningState(v1.ProvisioningStateFailed)
}

func finish(rec *datamodel.AWSResource, failure *v1.ErrorDetails) {
	rec.Properties.LastRequestToken = rec.Properties.RequestToken
	rec.Properties.LastRequestError = failure
	rec.Properties.RequestToken = ""
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
