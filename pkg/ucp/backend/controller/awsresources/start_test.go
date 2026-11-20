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

package awsresources

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"uuid"

	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/aws/schemas"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/native"
	"github.com/radius-project/radius/pkg/ucp/aws/native/nativefake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	startType = "AWS::Test::Native"
	startID   = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.Test/Native/app-native"
)

// startFixture has a record whose native operation waits for AWS to take its first call.
type startFixture struct {
	t     *testing.T
	db    database.Client
	store *records.Store
	id    resources.ID
	cc    *awsfake.CloudControl
	fake  *nativefake.Handler
	op    uuid.UUID
}

func newStartFixture(t *testing.T, op string) *startFixture {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	id, err := resources.ParseResource(startID)
	require.NoError(t, err)
	db := inmemory.NewClient()
	cc := awsfake.NewCloudControl()
	cc.Identifiers[startType] = "Name"
	f := &startFixture{t: t, db: db, store: records.NewStore(db), id: id, cc: cc, fake: nativefake.New(startType, cc), op: uuid.New()}
	f.fake.Opts.ReadyTimeout = time.Hour

	rec := records.NewRecord(id, "owner")
	rec.Properties.Handler = records.HandlerNative
	rec.Properties.RequestToken = f.op.String()
	rec.Properties.PendingOperation = op
	rec.Properties.StartState = records.StartWaiting
	rec.Properties.StartReason = nativefake.BusyReason
	rec.Properties.AWSIdentifier = "app-native"
	if op == records.OperationCreate {
		rec.Properties.ClientToken = "token"
	} else {
		cc.Put(startType, "app-native", map[string]any{"Name": "app-native", "Size": 1})
	}
	rec.SetProvisioningState(records.StateOf(op))
	_, err = f.store.Save(t.Context(), rec, "")
	require.NoError(t, err)
	return f
}

func (f *startFixture) clients() ucp_aws.Clients {
	f.t.Helper()
	registry, err := native.NewRegistry(schemas.Catalog{HandWritten: map[string]string{startType: `{"typeName": "AWS::Test::Native", "primaryIdentifier": ["/properties/Name"]}`}}, f.fake)
	require.NoError(f.t, err)
	return ucp_aws.Clients{CloudControl: f.cc, Native: registry}
}

// send runs sendStart with db for the operation op.
func (f *startFixture) send(ctx context.Context, db database.Client, op string) (*datamodel.AWSResource, ctrl.Result, error) {
	f.t.Helper()
	c, err := NewAwaitController(ctrl.Options{DatabaseClient: db}, f.clients(), v1.OperationPut)
	require.NoError(f.t, err)
	return c.(*AwaitController).sendStart(ctx, records.NewStore(db), f.id, &ctrl.Request{ResourceID: startID, OperationID: f.op}, op)
}

func (f *startFixture) record() *datamodel.AWSResource {
	f.t.Helper()
	rec, _, err := f.store.Get(f.t.Context(), f.id)
	require.NoError(f.t, err)
	return rec
}

// saveFails is a data store whose saves fail from the save numbered from, counting from 1, and whose reads run
// beforeRead, when set, before the read numbered at.
type saveFails struct {
	database.Client
	from       int32
	saves      *atomic.Int32
	at         int32
	reads      *atomic.Int32
	beforeRead func()
}

func newSaveFails(db database.Client, from int32) saveFails {
	return saveFails{Client: db, from: from, saves: &atomic.Int32{}, reads: &atomic.Int32{}}
}

func (d saveFails) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if d.beforeRead != nil && d.reads.Add(1) == d.at {
		d.beforeRead()
	}
	return d.Client.Get(ctx, id, options...)
}

func (d saveFails) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if d.from > 0 && d.saves.Add(1) >= d.from {
		return errors.New("database is down")
	}
	return d.Client.Save(ctx, obj, options...)
}

func Test_SendStart_RecordErrors(t *testing.T) {
	t.Run("the record can't be read", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		rec, result, err := f.send(t.Context(), brokenDB{Client: f.db, failGet: true}, records.OperationCreate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.True(t, result.Requeue)
		require.Empty(t, f.fake.Calls())
	})

	t.Run("a newer operation replaced it", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		f.op = uuid.New()
		rec, result, err := f.send(t.Context(), f.db, records.OperationCreate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
		require.True(t, result.KeepResourceState)
		require.Empty(t, f.fake.Calls())
	})

	t.Run("a request removes the record before the worker saves that it sends", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		db := newSaveFails(f.db, 0)
		db.at, db.beforeRead = 2, func() { require.NoError(t, f.db.Delete(t.Context(), startID)) }
		rec, result, err := f.send(t.Context(), db, records.OperationCreate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
		require.Empty(t, f.fake.Calls(), "the worker sends nothing")
	})

	// Each save that fails leaves the record as it was saved last, so the redelivered operation continues from there.
	saves := map[string]struct {
		from  int32
		busy  int
		state string
	}{
		"saving that it sends":       {from: 1, state: records.StartWaiting},
		"saving that AWS took it":    {from: 2, state: records.StartSending},
		"saving that it waits again": {from: 2, busy: 1, state: records.StartSending},
	}
	for name, tc := range saves {
		t.Run(name+" fails", func(t *testing.T) {
			f := newStartFixture(t, records.OperationCreate)
			f.fake.SetBusy(tc.busy)
			rec, result, err := f.send(t.Context(), newSaveFails(f.db, tc.from), records.OperationCreate)
			require.NoError(t, err)
			require.Nil(t, rec)
			require.True(t, result.Requeue)
			require.Equal(t, tc.state, f.record().Properties.StartState)
		})
	}

	t.Run("a request replaces it while AWS answers that it is busy", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.fake.SetBusy(1)
		f.fake.OnStart = func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			rec.Properties.RequestToken = uuid.New().String()
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		rec, result, err := f.send(t.Context(), f.db, records.OperationUpdate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	})

	t.Run("a request replaces it while AWS takes the call", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.fake.OnStart = func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			rec.Properties.RequestToken = uuid.New().String()
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		rec, result, err := f.send(t.Context(), f.db, records.OperationUpdate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	})
}

func Test_SendStart_Stops(t *testing.T) {
	t.Run("while the call is in flight", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		ctx, cancel := context.WithCancel(t.Context())
		f.fake.OnStart = cancel
		f.fake.Errors["Create"] = context.Canceled
		rec, _, err := f.send(ctx, f.db, records.OperationCreate)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, rec)
		require.Equal(t, records.StartSending, f.record().Properties.StartState, "the redelivered operation can't tell whether AWS took it")
	})

	t.Run("while it waits", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		ctx, cancel := context.WithCancel(t.Context())
		f.fake.SetBusy(-1)
		f.fake.OnStart = cancel
		rec, _, err := f.send(ctx, f.db, records.OperationCreate)
		require.ErrorIs(t, err, context.Canceled)
		require.Nil(t, rec)
		require.Equal(t, records.StartWaiting, f.record().Properties.StartState)
	})
}

func Test_SendStart_Delete(t *testing.T) {
	t.Run("gone", func(t *testing.T) {
		f := newStartFixture(t, records.OperationDelete)
		f.cc.Remove(startType, "app-native")
		rec, result, err := f.send(t.Context(), f.db, records.OperationDelete)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Nil(t, result.Error)
		require.Nil(t, f.record())
	})

	t.Run("gone, and the record can't be removed", func(t *testing.T) {
		f := newStartFixture(t, records.OperationDelete)
		f.cc.Remove(startType, "app-native")
		rec, result, err := f.send(t.Context(), brokenDelete{Client: f.db}, records.OperationDelete)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.True(t, result.Requeue)
	})

	t.Run("gone, and a request replaced it", func(t *testing.T) {
		f := newStartFixture(t, records.OperationDelete)
		f.cc.Remove(startType, "app-native")
		f.fake.OnStart = func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			rec.Properties.RequestToken = uuid.New().String()
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		_, result, err := f.send(t.Context(), f.db, records.OperationDelete)
		require.NoError(t, err)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	})
}

// brokenDelete is a data store whose deletes fail.
type brokenDelete struct {
	database.Client
}

func (b brokenDelete) Delete(ctx context.Context, id string, options ...database.DeleteOptions) error {
	return errors.New("database is down")
}

func Test_SendStart_Failures(t *testing.T) {
	t.Run("no native handler while the call may be in flight", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.store = records.NewStore(f.db)
		rec, etag, err := f.store.Get(t.Context(), f.id)
		require.NoError(t, err)
		rec.Properties.StartState = records.StartSending
		_, err = f.store.Save(t.Context(), rec, etag)
		require.NoError(t, err)

		c, err := NewAwaitController(ctrl.Options{DatabaseClient: f.db}, ucp_aws.Clients{CloudControl: f.cc}, v1.OperationPut)
		require.NoError(t, err)
		_, result, err := c.(*AwaitController).sendStart(t.Context(), f.store, f.id, &ctrl.Request{ResourceID: startID, OperationID: f.op}, records.OperationUpdate)
		require.NoError(t, err)
		require.True(t, strings.Contains(result.Error.Message, "UCP has no native handler"))
		require.Equal(t, v1.ProvisioningStateFailed, f.record().ProvisioningState())
	})

	t.Run("no native handler while the operation waits", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		c, err := NewAwaitController(ctrl.Options{DatabaseClient: f.db}, ucp_aws.Clients{CloudControl: f.cc}, v1.OperationPut)
		require.NoError(t, err)
		_, result, err := c.(*AwaitController).sendStart(t.Context(), f.store, f.id, &ctrl.Request{ResourceID: startID, OperationID: f.op}, records.OperationCreate)
		require.NoError(t, err)
		require.Contains(t, result.Error.Message, "UCP has no native handler")
		requireNoResource(t, f.record())
	})

	t.Run("settling the failure finds a newer operation", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.fake.Errors["Update"] = nativefake.ClientError("ValidationException", "bad size")
		f.fake.OnStart = func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			rec.Properties.StartState = records.StartSending
			rec.Properties.RequestToken = uuid.New().String()
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		_, result, err := f.send(t.Context(), f.db, records.OperationUpdate)
		require.NoError(t, err)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	})

	t.Run("settling the failure can't save", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.fake.Errors["Update"] = nativefake.ClientError("ValidationException", "bad size")
		_, result, err := f.send(t.Context(), newSaveFails(f.db, 2), records.OperationUpdate)
		require.NoError(t, err)
		require.True(t, result.Requeue)
	})

	t.Run("a create with no properties", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		rec, _, err := f.send(t.Context(), f.db, records.OperationCreate)
		require.NoError(t, err)
		require.NotNil(t, rec)
		require.Equal(t, map[string]any{}, f.fake.Calls()[0].Properties)
	})
}

// run runs the operation with the worker, as the async worker does, with the native operation type.
func (f *startFixture) run(method v1.OperationMethod, db database.Client) ctrl.Result {
	f.t.Helper()
	c, err := NewAwaitController(ctrl.Options{DatabaseClient: db}, f.clients(), method)
	require.NoError(f.t, err)
	result, err := c.Run(f.t.Context(), &ctrl.Request{ResourceID: startID, OperationID: f.op, OperationType: records.NativeOperationType + "|" + string(method)})
	require.NoError(f.t, err)
	return result
}

func Test_Run_WaitingStart(t *testing.T) {
	t.Run("sends the call once AWS takes it, then waits for it", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		rec, etag, err := f.store.Get(t.Context(), f.id)
		require.NoError(t, err)
		rec.Properties.PendingLastApplied = map[string]any{"Name": "app-native", "Size": 2}
		rec.Properties.StartPatch = []map[string]any{{"op": "replace", "path": "/Size", "value": 2}}
		rec.Properties.UpdateToken = "update-token"
		_, err = f.store.Save(t.Context(), rec, etag)
		require.NoError(t, err)
		f.fake.SetBusy(2)

		result := f.run(v1.OperationPut, f.db)
		require.Nil(t, result.Error)
		calls := f.fake.Calls()
		require.Len(t, calls, 3)
		for _, c := range calls {
			require.Equal(t, "update-token", c.Token)
		}
		rec = f.record()
		require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
		require.Empty(t, rec.Properties.StartState)
		require.EqualValues(t, 2, f.cc.Resource(startType, "app-native")["Size"])
	})

	t.Run("ReadyTimeout passes", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		f.fake.Opts.ReadyTimeout = time.Nanosecond
		result := f.run(v1.OperationPut, f.db)
		require.Equal(t, `UCP waited 1ns for AWS to take the create of the AWS resource "app-native", because the parent "parent-1" is busy. Nothing was sent to AWS. Deploy again.`, result.Error.Message)
		requireNoResource(t, f.record())
		require.Empty(t, f.fake.Calls())
	})

	t.Run("an update the worker may have sent before it stopped", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		rec, etag, err := f.store.Get(t.Context(), f.id)
		require.NoError(t, err)
		rec.Properties.StartState = records.StartSending
		rec.Properties.UpdateToken = "update-token"
		_, err = f.store.Save(t.Context(), rec, etag)
		require.NoError(t, err)

		result := f.run(v1.OperationPut, f.db)
		require.Equal(t, `UCP stopped while it sent the update of the AWS resource "app-native", so it can't tell whether AWS took it. Deploy again to continue.`, result.Error.Message)
		require.Empty(t, f.fake.Calls())
		require.Equal(t, "update-token", f.record().Properties.UpdateToken)
	})

	t.Run("AWS rejects the update", func(t *testing.T) {
		f := newStartFixture(t, records.OperationUpdate)
		f.fake.Errors["Update"] = nativefake.ClientError("ValidationException", "bad size")
		result := f.run(v1.OperationPut, f.db)
		require.Equal(t, v1.CodeInvalid, result.Error.Code)
		rec := f.record()
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
		require.Empty(t, rec.Properties.StartState)
	})

	t.Run("the generated name doesn't fit", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		f.fake.Opts.NameProperty = "Name"
		registry, err := native.NewRegistry(schemas.Catalog{HandWritten: map[string]string{startType: `{"typeName": "AWS::Test::Native", "properties": {"Name": {"type": "string"}}, "primaryIdentifier": ["/properties/Name"]}`}}, f.fake)
		require.NoError(t, err)
		registry.Lookup(startType).NameRules.MinLength = 100
		c, err := NewAwaitController(ctrl.Options{DatabaseClient: f.db}, ucp_aws.Clients{CloudControl: f.cc, Native: registry}, v1.OperationPut)
		require.NoError(t, err)
		result, err := c.Run(t.Context(), &ctrl.Request{ResourceID: startID, OperationID: f.op})
		require.NoError(t, err)
		require.Equal(t, v1.CodeInvalid, result.Error.Code)
		require.Contains(t, result.Error.Message, "doesn't fit the rules of Name")
		require.Empty(t, f.fake.Calls())
	})
}

func Test_Run_Replaced(t *testing.T) {
	t.Run("a create whose record a request removed", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		require.NoError(t, f.db.Delete(t.Context(), startID))
		result := f.run(v1.OperationPut, f.db)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
		require.True(t, result.KeepResourceState)
	})

	t.Run("a delete whose record is gone is done", func(t *testing.T) {
		f := newStartFixture(t, records.OperationDelete)
		require.NoError(t, f.db.Delete(t.Context(), startID))
		result := f.run(v1.OperationDelete, f.db)
		require.Nil(t, result.Error)
	})

	// The async worker marks the record Updating when it picks up the replaced operation, after the request that
	// replaced it finished without one.
	for name, db := range map[string]func(f *startFixture) database.Client{
		"the record is released":           func(f *startFixture) database.Client { return f.db },
		"releasing the record fails":       func(f *startFixture) database.Client { return newSaveFails(f.db, 1) },
		"a request changes the record too": nil,
	} {
		t.Run(name, func(t *testing.T) {
			f := newStartFixture(t, records.OperationDelete)
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			records.DropWaiting(rec)
			rec.Properties.RequestToken = ""
			rec.SetProvisioningState(v1.ProvisioningStateUpdating)
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)

			var store database.Client
			if db != nil {
				store = db(f)
			}
			if name == "a request changes the record too" {
				store = racingSave{Client: f.db, before: func() {
					rec, etag, err := f.store.Get(t.Context(), f.id)
					require.NoError(t, err)
					_, err = f.store.Save(t.Context(), rec, etag)
					require.NoError(t, err)
				}}
			}
			result := f.run(v1.OperationDelete, store)
			require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
			want := v1.ProvisioningStateCanceled
			if name != "the record is released" {
				want = v1.ProvisioningStateUpdating
			}
			require.Equal(t, want, f.record().ProvisioningState())
		})
	}
}

// racingSave is a data store where another request saves the record just before each save, so the save loses the
// race.
type racingSave struct {
	database.Client
	before func()
}

func (r racingSave) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	r.before()
	return r.Client.Save(ctx, obj, options...)
}

// removeBeforeSave is a data store where a request removes the record just before the first save, so that save loses
// the race and the retry finds no record.
type removeBeforeSave struct {
	database.Client
	once *atomic.Bool
}

func (r removeBeforeSave) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if r.once.CompareAndSwap(false, true) {
		if err := r.Client.Delete(ctx, obj.ID); err != nil {
			return err
		}
	}
	return r.Client.Save(ctx, obj, options...)
}

func Test_SendStart_RecordRemovedDuringSave(t *testing.T) {
	f := newStartFixture(t, records.OperationCreate)
	rec, result, err := f.send(t.Context(), removeBeforeSave{Client: f.db, once: &atomic.Bool{}}, records.OperationCreate)
	require.NoError(t, err)
	require.Nil(t, rec)
	require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
	require.Empty(t, f.fake.Calls(), "the worker never sends a create whose record a request removed")
	require.Nil(t, f.record())
}

// Test_SendStart_OverlappingDeliveries runs a second delivery of the operation, as when the queue hands it out again
// while the first still runs. Only one delivery can send the call.
func Test_SendStart_OverlappingDeliveries(t *testing.T) {
	t.Run("the other delivery claims the call after this one read it waiting", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		db := newSaveFails(f.db, 0)
		// The second read is the one before this delivery saves that it sends the call.
		db.at, db.beforeRead = 2, func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			rec.Properties.StartState = records.StartSending
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		rec, result, err := f.send(t.Context(), db, records.OperationCreate)
		require.NoError(t, err)
		require.Nil(t, rec)
		require.Empty(t, f.fake.Calls(), "only the delivery that claimed the call sends it")
		require.Contains(t, result.Error.Message, "can't tell whether AWS took it")
		kept := f.record()
		require.NotEmpty(t, kept.Properties.ClientToken, "the next deploy finds the create the other delivery sent")
		require.False(t, records.IsWaiting(kept))
	})

	t.Run("the other delivery already sent the call AWS took", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		db := newSaveFails(f.db, 0)
		db.at, db.beforeRead = 2, func() {
			rec, etag, err := f.store.Get(t.Context(), f.id)
			require.NoError(t, err)
			records.ClearStart(rec)
			_, err = f.store.Save(t.Context(), rec, etag)
			require.NoError(t, err)
		}
		rec, _, err := f.send(t.Context(), db, records.OperationCreate)
		require.NoError(t, err)
		require.NotNil(t, rec, "this delivery waits for the call the other one sent")
		require.Empty(t, f.fake.Calls())
	})

	t.Run("a late busy answer never makes a call AWS took replaceable", func(t *testing.T) {
		f := newStartFixture(t, records.OperationCreate)
		f.fake.SetBusy(1)
		f.fake.OnStart = func() {
			f.fake.OnStart = nil
			// The other delivery finds this one's claim, and ends the operation as unknown.
			_, result, err := f.send(t.Context(), f.db, records.OperationCreate)
			require.NoError(t, err)
			require.NotNil(t, result.Error)
		}
		_, result, err := f.send(t.Context(), f.db, records.OperationCreate)
		require.NoError(t, err)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
		require.False(t, records.IsWaiting(f.record()))
	})
}

// Test_Run_FailedCreateRedelivered runs a waiting create the worker fails again, as when the worker stops after it
// saves the failure and before it saves the operation's status. The redelivered operation reports the same failure.
func Test_Run_FailedCreateRedelivered(t *testing.T) {
	cases := map[string]func(f *startFixture){
		"ReadyTimeout passes": func(f *startFixture) { f.fake.Opts.ReadyTimeout = time.Nanosecond },
		"AWS rejects it": func(f *startFixture) {
			f.fake.Errors["Create"] = nativefake.ClientError("ValidationException", "bad size")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			f := newStartFixture(t, records.OperationCreate)
			setup(f)
			first := f.run(v1.OperationPut, f.db)
			require.NotNil(t, first.Error)

			again := f.run(v1.OperationPut, f.db)
			require.Equal(t, v1.ProvisioningStateFailed, again.ProvisioningState())
			require.Equal(t, first.Error.Message, again.Error.Message)

			rec := f.record()
			require.Empty(t, rec.Properties.ClientToken, "nothing of the create is in AWS")
			require.Empty(t, rec.Properties.AWSIdentifier, "no other alias is kept from the identifier")
			require.Nil(t, rec.Properties.PendingLastApplied)
		})
	}
}

// requireNoResource requires a failed record that holds no resource, as the worker leaves after a first create AWS never
// had.
func requireNoResource(t *testing.T, rec *datamodel.AWSResource) {
	t.Helper()
	require.NotNil(t, rec)
	require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
	require.Empty(t, rec.Properties.ClientToken)
	require.Empty(t, rec.Properties.AWSIdentifier)
	require.Empty(t, rec.Properties.RequestToken)
	require.NotNil(t, rec.Properties.LastRequestError)
	require.False(t, records.IsWaiting(rec))
}

func Test_SendStart_DeleteFullyApplied(t *testing.T) {
	f := newStartFixture(t, records.OperationDelete)
	f.fake.LostResponses["Delete"] = &native.Failure{Message: "removing a tag failed after the delete", Outcome: native.OutcomeApplied}
	_, result, err := f.send(t.Context(), f.db, records.OperationDelete)
	require.NoError(t, err)
	require.Equal(t, "removing a tag failed after the delete", result.Error.Message)
	require.Nil(t, f.record(), "a delete AWS fully applied removes the record")
}

func Test_ClaimTaken_Error(t *testing.T) {
	require.EqualError(t, &claimTaken{state: records.StartSending}, `another delivery of the operation changed its start state to "sending"`)
}

// Test_SendStart_TimeoutWhileAnotherDeliverySends runs out the wait of one delivery while another delivery of the
// operation sends the call. The create may reach AWS, so the record keeps what finds it.
func Test_SendStart_TimeoutWhileAnotherDeliverySends(t *testing.T) {
	f := newStartFixture(t, records.OperationCreate)
	f.fake.Opts.ReadyTimeout = time.Nanosecond
	db := newSaveFails(f.db, 0)
	// The second read is the one the timeout settles the record with.
	db.at, db.beforeRead = 2, func() {
		rec, etag, err := f.store.Get(t.Context(), f.id)
		require.NoError(t, err)
		rec.Properties.StartState = records.StartSending
		_, err = f.store.Save(t.Context(), rec, etag)
		require.NoError(t, err)
	}
	_, result, err := f.send(t.Context(), db, records.OperationCreate)
	require.NoError(t, err)
	require.Contains(t, result.Error.Message, "can't tell whether AWS took it")
	rec := f.record()
	require.Equal(t, "token", rec.Properties.ClientToken, "the next request finds the create the other delivery sent")
	require.Equal(t, "app-native", rec.Properties.AWSIdentifier)
}

// Test_Run_FailedCreateAfterResourceGone fails a waiting create of an alias whose earlier resource was deleted outside
// Radius. The record keeps nothing of the earlier resource, as a request that removes it would.
func Test_Run_FailedCreateAfterResourceGone(t *testing.T) {
	f := newStartFixture(t, records.OperationCreate)
	rec, etag, err := f.store.Get(t.Context(), f.id)
	require.NoError(t, err)
	rec.Properties.LastApplied = map[string]any{"Tags": []any{"team"}}
	rec.Properties.UnresolvedPaths = []string{"Tags"}
	rec.Properties.Fingerprints = &datamodel.AWSFingerprints{KeyID: "key", Digests: map[string]string{"Password": "d"}}
	rec.Properties.CreateToken = "earlier"
	rec.Properties.DeleteToken = "final-1"
	_, err = f.store.Save(t.Context(), rec, etag)
	require.NoError(t, err)
	f.fake.Opts.ReadyTimeout = time.Nanosecond

	result := f.run(v1.OperationPut, f.db)
	require.NotNil(t, result.Error)
	kept := f.record()
	requireNoResource(t, kept)
	require.Nil(t, kept.Properties.LastApplied, "the alias owns nothing of the next resource it takes over")
	require.Empty(t, kept.Properties.UnresolvedPaths)
	require.Nil(t, kept.Properties.Fingerprints)
	require.Empty(t, kept.Properties.CreateToken)
	require.Empty(t, kept.Properties.DeleteToken)
	require.Equal(t, "owner", kept.Properties.Owner)
	require.Equal(t, records.HandlerNative, kept.Properties.Handler)
}
