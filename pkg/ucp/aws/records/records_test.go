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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	vpcID   = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/VPC/app-vpc"
	otherID = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/VPC/other-vpc"
)

func parse(t *testing.T, id string) resources.ID {
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)
	return parsed
}

func Test_Store(t *testing.T) {
	store := NewStore(inmemory.NewClient())
	id := parse(t, vpcID)

	rec, etag, err := store.Get(t.Context(), id)
	require.NoError(t, err)
	require.Nil(t, rec)
	require.Empty(t, etag)

	rec = NewRecord(id, "owner")
	rec.Properties.AWSIdentifier = "vpc-1"
	etag, err = store.Save(t.Context(), rec, "")
	require.NoError(t, err)
	require.NotEmpty(t, etag)

	// A second first save of the same alias loses.
	_, err = store.Save(t.Context(), NewRecord(id, "other"), "")
	require.ErrorIs(t, err, &database.ErrConcurrency{})

	got, gotETag, err := store.Get(t.Context(), id)
	require.NoError(t, err)
	require.Equal(t, etag, gotETag)
	require.Equal(t, "app-vpc", got.Name)
	require.Equal(t, "AWS.EC2/VPC", got.ResourceTypeName())
	require.Equal(t, "owner", got.Properties.Owner)
	require.False(t, LastModified(got).IsZero())

	// A save with a stale ETag loses.
	_, err = store.Save(t.Context(), got, "stale")
	require.ErrorIs(t, err, &database.ErrConcurrency{})

	require.NoError(t, store.Touch(t.Context(), id))
	require.NoError(t, store.Touch(t.Context(), parse(t, otherID)))

	require.NoError(t, store.Delete(t.Context(), id))
	require.NoError(t, store.Delete(t.Context(), id))
}

func Test_Store_Errors(t *testing.T) {
	store := NewStore(failing{Client: inmemory.NewClient()})
	_, _, err := store.Get(t.Context(), parse(t, vpcID))
	require.ErrorContains(t, err, "failed to read the record")
	_, err = store.FindByIdentifier(t.Context(), parse(t, vpcID), "vpc-1")
	require.ErrorContains(t, err, "failed to search the records")
	require.Error(t, store.Touch(t.Context(), parse(t, vpcID)))

	// Data that is not a record is an error, not "no record".
	db := inmemory.NewClient()
	require.NoError(t, db.Save(t.Context(), &database.Object{Metadata: database.Metadata{ID: vpcID}, Data: map[string]any{"properties": "x"}}))
	_, _, err = NewStore(db).Get(t.Context(), parse(t, vpcID))
	require.ErrorContains(t, err, "failed to read the record")
}

func Test_FindByIdentifier(t *testing.T) {
	db := inmemory.NewClient()
	store := NewStore(db)
	ctx := t.Context()

	holder, err := store.FindByIdentifier(ctx, parse(t, otherID), "")
	require.NoError(t, err)
	require.Empty(t, holder)

	rec := NewRecord(parse(t, vpcID), "owner")
	rec.Properties.AWSIdentifier = "vpc-1"
	_, err = store.Save(ctx, rec, "")
	require.NoError(t, err)

	holder, err = store.FindByIdentifier(ctx, parse(t, otherID), "vpc-1")
	require.NoError(t, err)
	require.Equal(t, vpcID, holder)
	require.Equal(t, "app-vpc", AliasOf(holder))

	// The alias's own record does not count.
	holder, err = store.FindByIdentifier(ctx, parse(t, vpcID), "vpc-1")
	require.NoError(t, err)
	require.Empty(t, holder)

	// Other types and regions do not count.
	holder, err = store.FindByIdentifier(ctx, parse(t, "/planes/aws/aws/accounts/1234567/regions/us-east-1/providers/AWS.EC2/VPC/x"), "vpc-1")
	require.NoError(t, err)
	require.Empty(t, holder)
	holder, err = store.FindByIdentifier(ctx, parse(t, "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/Subnet/x"), "vpc-1")
	require.NoError(t, err)
	require.Empty(t, holder)
}

func Test_FindByIdentifier_Pages(t *testing.T) {
	store := NewStore(paged{Client: inmemory.NewClient()})
	holder, err := store.FindByIdentifier(t.Context(), parse(t, otherID), "vpc-1")
	require.NoError(t, err)
	require.Equal(t, vpcID, holder)
}

func Test_State(t *testing.T) {
	rec := NewRecord(parse(t, vpcID), "owner")
	require.False(t, IsRunning(rec))
	require.True(t, IsStale(rec))
	require.False(t, HasApplied(rec))
	require.Equal(t, OperationUpdate, PendingOperation(rec))

	rec.Properties.ClientToken = "token"
	require.True(t, IsFirstCreate(rec))
	require.Equal(t, OperationCreate, PendingOperation(rec))
	rec.SetProvisioningState(v1.ProvisioningStateDeleting)
	require.Equal(t, OperationDelete, PendingOperation(rec))
	require.True(t, IsRunning(rec))
	rec.Properties.PendingOperation = OperationUpdate
	require.Equal(t, OperationUpdate, PendingOperation(rec))

	rec.SystemData.LastModifiedAt = time.Now().UTC().Format(time.RFC3339Nano)
	require.False(t, IsStale(rec))
	defer func() { Now = time.Now }()
	Now = func() time.Time { return time.Now().Add(StaleAfter + time.Minute) }
	require.True(t, IsStale(rec))

	for op, state := range map[string]v1.ProvisioningState{
		OperationCreate: v1.ProvisioningStateProvisioning,
		OperationUpdate: v1.ProvisioningStateUpdating,
		OperationDelete: v1.ProvisioningStateDeleting,
	} {
		require.Equal(t, state, StateOf(op))
	}
	require.Equal(t, "not-an-id", AliasOf("not-an-id"))
}

func Test_ApplyPendingAndFail(t *testing.T) {
	newRec := func() *datamodel.AWSResource {
		rec := NewRecord(parse(t, vpcID), "owner")
		rec.Properties.LastApplied = map[string]any{"A": "1"}
		rec.Properties.PendingLastApplied = map[string]any{"A": "2"}
		rec.Properties.PendingOperation = OperationUpdate
		rec.Properties.ClientToken = "c"
		rec.Properties.RequestToken = "r"
		return rec
	}

	rec := newRec()
	ApplyPending(rec)
	require.Equal(t, map[string]any{"A": "2"}, rec.Properties.LastApplied)
	require.Nil(t, rec.Properties.PendingLastApplied)
	require.Empty(t, rec.Properties.PendingOperation+rec.Properties.ClientToken+rec.Properties.RequestToken)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
	require.True(t, HasApplied(rec))

	rec = newRec()
	Fail(rec)
	require.Equal(t, map[string]any{"A": "1"}, rec.Properties.LastApplied)
	require.Nil(t, rec.Properties.PendingLastApplied)
	require.Empty(t, rec.Properties.RequestToken)
	require.Equal(t, "c", rec.Properties.ClientToken)
	require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())

	// A successful operation without pending values, such as a resumed one, keeps the last-applied properties.
	rec = newRec()
	rec.Properties.PendingLastApplied = nil
	ApplyPending(rec)
	require.Equal(t, map[string]any{"A": "1"}, rec.Properties.LastApplied)
}

type failing struct{ database.Client }

func (failing) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	return nil, errors.New("down")
}

func (failing) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	return nil, errors.New("down")
}

// paged returns the alias's own record on the first page and the holder on the second.
type paged struct{ database.Client }

func (paged) Query(ctx context.Context, query database.Query, options ...database.QueryOptions) (*database.ObjectQueryResult, error) {
	if database.NewQueryConfig(options...).PaginationToken == "" {
		return &database.ObjectQueryResult{Items: []database.Object{{Metadata: database.Metadata{ID: otherID}}}, PaginationToken: "next"}, nil
	}
	return &database.ObjectQueryResult{Items: []database.Object{{Metadata: database.Metadata{ID: vpcID}}}}, nil
}
