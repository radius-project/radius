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

package records

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// Store reads and writes records and operations in the Radius database.
type Store struct {
	DB  database.Client
	Now func() time.Time
}

// NewStore returns a Store backed by db.
func NewStore(db database.Client) *Store {
	return &Store{DB: db, Now: time.Now}
}

func (s *Store) now() time.Time {
	if s.Now == nil {
		return time.Now()
	}
	return s.Now()
}

// NewRecord fills the defaults of a new active record.
func (s *Store) NewRecord(r Record) *Record {
	now := s.now()
	if r.ID == "" {
		r.ID = NewRecordID()
	}
	r.Status = RecordActive
	r.CreatedAt = now
	if r.ExpiresAt.IsZero() {
		r.ExpiresAt = now.Add(DefaultRecordTTL)
	}
	return &r
}

// SaveRecord saves r. etag is the expected ETag, or "" for an unconditional save.
func (s *Store) SaveRecord(ctx context.Context, r *Record, etag string) (string, error) {
	obj := &database.Object{Metadata: database.Metadata{ID: r.ID}, Data: r}
	if err := s.DB.Save(ctx, obj, database.WithETag(etag)); err != nil {
		return "", err
	}
	return obj.ETag, nil
}

// GetRecord returns the record with id.
func (s *Store) GetRecord(ctx context.Context, id string) (*Record, string, error) {
	id = qualify(id, RecordResourceType)
	if _, err := resources.Parse(id); err != nil {
		return nil, "", &database.ErrNotFound{ID: id}
	}
	obj, err := s.DB.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	r := &Record{}
	if err := obj.As(r); err != nil {
		return nil, "", err
	}
	return r, obj.ETag, nil
}

// ListRecords returns every stored record.
func (s *Store) ListRecords(ctx context.Context) ([]*Record, error) {
	result, err := s.DB.Query(ctx, database.Query{RootScope: RootScope, ResourceType: RecordResourceType})
	if err != nil {
		return nil, err
	}
	out := make([]*Record, 0, len(result.Items))
	for _, item := range result.Items {
		r := &Record{}
		if err := item.As(r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ActiveRecords returns the active records whose submitter is submitter.
func (s *Store) ActiveRecords(ctx context.Context, submitter string) ([]*Record, error) {
	all, err := s.ListRecords(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	out := []*Record{}
	for _, r := range all {
		if r.Active(now) && (submitter == "" || r.Submitter == submitter) {
			out = append(out, r)
		}
	}
	return out, nil
}

// CloseRecord closes the record with id and any active children. Closing a closed
// record is a no-op.
func (s *Store) CloseRecord(ctx context.Context, id string, status RecordStatus) error {
	all, err := s.ListRecords(ctx)
	if err != nil {
		return err
	}
	return s.close(ctx, all, id, status)
}

func (s *Store) close(ctx context.Context, all []*Record, id string, status RecordStatus) error {
	r, etag, err := s.GetRecord(ctx, id)
	if errors.Is(err, &database.ErrNotFound{}) {
		return nil
	} else if err != nil {
		return err
	}
	if r.Status == RecordActive {
		now := s.now()
		r.Status = status
		r.ClosedAt = &now
		if _, err := s.SaveRecord(ctx, r, etag); err != nil && !errors.Is(err, &database.ErrConcurrency{}) {
			return err
		}
	}
	for _, child := range all {
		if SameResource(child.ParentID, id) && child.Status == RecordActive {
			if err := s.close(ctx, all, child.ID, status); err != nil {
				return err
			}
		}
	}
	return nil
}

// CreateOperation saves a new queued operation.
func (s *Store) CreateOperation(ctx context.Context, op *Operation) error {
	if op.ID == "" {
		op.ID = NewOperationID()
	}
	if op.Status == "" {
		op.Status = OperationQueued
	}
	op.CreatedAt = s.now()
	_, err := s.SaveOperation(ctx, op, "")
	return err
}

// SaveOperation saves op. etag is the expected ETag, or "" for an unconditional save.
func (s *Store) SaveOperation(ctx context.Context, op *Operation, etag string) (string, error) {
	obj := &database.Object{Metadata: database.Metadata{ID: op.ID}, Data: op}
	if err := s.DB.Save(ctx, obj, database.WithETag(etag)); err != nil {
		return "", err
	}
	return obj.ETag, nil
}

// GetOperation returns the operation with id.
func (s *Store) GetOperation(ctx context.Context, id string) (*Operation, string, error) {
	id = qualify(id, OperationResourceType)
	if _, err := resources.Parse(id); err != nil {
		return nil, "", &database.ErrNotFound{ID: id}
	}
	obj, err := s.DB.Get(ctx, id)
	if err != nil {
		return nil, "", err
	}
	op := &Operation{}
	if err := obj.As(op); err != nil {
		return nil, "", err
	}
	return op, obj.ETag, nil
}

// FinishOperation moves an open operation to status. It ignores missing and closed operations.
func (s *Store) FinishOperation(ctx context.Context, id string, status OperationStatus) error {
	for range 3 {
		op, etag, err := s.GetOperation(ctx, id)
		if errors.Is(err, &database.ErrNotFound{}) {
			return nil
		} else if err != nil {
			return err
		}
		if !op.Open() {
			return nil
		}
		op.Status = status
		op.LeaseHolder = ""
		op.LeaseExpiresAt = nil
		_, err = s.SaveOperation(ctx, op, etag)
		if errors.Is(err, &database.ErrConcurrency{}) {
			continue
		}
		return err
	}
	return &database.ErrConcurrency{}
}

// ErrClaimed is returned by Claim when another holder has a live lease.
var ErrClaimed = errors.New("operation is claimed by another worker")

// Claim moves a queued operation to running with a lease for holder. It uses the ETag so
// only one worker wins. A running operation may be re-claimed by the same holder, or by
// anyone once the lease expires.
func (s *Store) Claim(ctx context.Context, id, holder string) (*Operation, error) {
	op, etag, err := s.GetOperation(ctx, id)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if op.Status == OperationRunning && op.LeaseHolder != holder && op.LeaseExpiresAt != nil && now.Before(*op.LeaseExpiresAt) {
		return op, ErrClaimed
	}
	if !op.Open() {
		return op, nil
	}
	expires := now.Add(DefaultLease)
	op.Status = OperationRunning
	op.LeaseHolder = holder
	op.LeaseExpiresAt = &expires
	if _, err := s.SaveOperation(ctx, op, etag); err != nil {
		if errors.Is(err, &database.ErrConcurrency{}) {
			return op, ErrClaimed
		}
		return nil, err
	}
	return op, nil
}

// CurrentTime returns the store's current time.
func (s *Store) CurrentTime() time.Time {
	return s.now()
}

// qualify turns a bare record or operation name into its full resource ID.
func qualify(id, resourceType string) string {
	if id == "" || strings.HasPrefix(id, "/") {
		return id
	}
	return RootScope + "/providers/" + resourceType + "/" + id
}
