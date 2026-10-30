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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"uuid"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/stretchr/testify/require"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/inmemory"
	ucp_aws "github.com/radius-project/radius/pkg/ucp/aws"
	"github.com/radius-project/radius/pkg/ucp/aws/awsfake"
	"github.com/radius-project/radius/pkg/ucp/aws/records"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	vpcType = "AWS::EC2::VPC"
	vpcID   = "/planes/aws/aws/accounts/1234567/regions/us-west-2/providers/AWS.EC2/VPC/app-vpc"
)

type fixture struct {
	t     *testing.T
	cc    *awsfake.CloudControl
	db    database.Client
	store *records.Store
	id    resources.ID
	// op is the operation ID of the last accepted request: its Cloud Control request token.
	op uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	ucp_aws.MinPollInterval, ucp_aws.MaxPollInterval = time.Millisecond, time.Millisecond
	db := inmemory.NewClient()
	id, err := resources.ParseResource(vpcID)
	require.NoError(t, err)
	useClock(t)
	return &fixture{t: t, cc: awsfake.NewCloudControl(), db: db, store: records.NewStore(db), id: id}
}

// accept starts a create in AWS and saves the record the frontend would.
func (f *fixture) accept(op string) {
	f.t.Helper()
	out, err := f.cc.CreateResource(f.t.Context(), &cloudcontrol.CreateResourceInput{TypeName: aws.String(vpcType), DesiredState: aws.String(`{"CidrBlock":"10.0.0.0/16"}`)})
	require.NoError(f.t, err)
	rec := records.NewRecord(f.id, "owner")
	rec.Properties.RequestToken = aws.ToString(out.ProgressEvent.RequestToken)
	f.op = uuid.MustParse(rec.Properties.RequestToken)
	rec.Properties.PendingOperation = op
	rec.Properties.PendingLastApplied = map[string]any{"CidrBlock": "10.0.0.0/16"}
	if op == records.OperationCreate {
		rec.Properties.ClientToken = "token"
	} else {
		rec.Properties.AWSIdentifier = "id-1"
		rec.Properties.LastApplied = map[string]any{}
	}
	rec.SetProvisioningState(records.StateOf(op))
	_, etag, err := f.store.Get(f.t.Context(), f.id)
	require.NoError(f.t, err)
	_, err = f.store.Save(f.t.Context(), rec, etag)
	require.NoError(f.t, err)
}

func (f *fixture) run(method v1.OperationMethod) ctrl.Result {
	f.t.Helper()
	return f.runWith(f.t.Context(), method, f.db)
}

func (f *fixture) runWith(ctx context.Context, method v1.OperationMethod, db database.Client) ctrl.Result {
	f.t.Helper()
	c, err := NewAwaitController(ctrl.Options{DatabaseClient: db}, ucp_aws.Clients{CloudControl: f.cc}, method)
	require.NoError(f.t, err)
	result, _ := c.Run(ctx, &ctrl.Request{ResourceID: vpcID, OperationID: f.op})
	return result
}

func (f *fixture) record() *datamodel.AWSResource {
	f.t.Helper()
	rec, _, err := f.store.Get(f.t.Context(), f.id)
	require.NoError(f.t, err)
	return rec
}

func Test_Await_CreateSucceeds(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationCreate)
	result := f.run(v1.OperationPut)
	require.Nil(t, result.Error)
	require.False(t, result.Requeue)

	rec := f.record()
	require.Equal(t, "id-1", rec.Properties.AWSIdentifier)
	require.Equal(t, map[string]any{"CidrBlock": "10.0.0.0/16"}, rec.Properties.LastApplied)
	require.Nil(t, rec.Properties.PendingLastApplied)
	require.Empty(t, rec.Properties.ClientToken)
	require.Empty(t, rec.Properties.RequestToken)
	require.Empty(t, rec.Properties.PendingOperation)
	require.Equal(t, v1.ProvisioningStateSucceeded, rec.ProvisioningState())
}

func Test_Await_Failures(t *testing.T) {
	t.Run("failed create removes the record", func(t *testing.T) {
		f := newFixture(t)
		f.cc.FailNext = "InvalidRequest"
		f.accept(records.OperationCreate)
		result := f.run(v1.OperationPut)
		require.Equal(t, v1.CodeInvalid, result.Error.Code)
		require.Nil(t, f.record())
	})

	t.Run("failed update keeps the previous values", func(t *testing.T) {
		f := newFixture(t)
		f.cc.FailNext = "InternalFailure"
		f.accept(records.OperationUpdate)
		result := f.run(v1.OperationPut)
		require.Equal(t, v1.CodeInternal, result.Error.Code)
		rec := f.record()
		require.Equal(t, map[string]any{}, rec.Properties.LastApplied)
		require.Nil(t, rec.Properties.PendingLastApplied)
		require.Equal(t, v1.ProvisioningStateFailed, rec.ProvisioningState())
	})

	t.Run("Cloud Control forgot the request", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		f.saveWith(func(rec *datamodel.AWSResource) { rec.Properties.RequestToken = "00000000-0000-0000-0000-000000000000" })
		f.op = uuid.UUID{}
		result := f.run(v1.OperationPut)
		require.Contains(t, result.Error.Message, "no longer has the request")
		require.Empty(t, f.record().Properties.RequestToken)
	})

	t.Run("polling fails, so the operation is retried", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		f.cc.Errors["GetResourceRequestStatus"] = errors.New("throttled")
		result := f.run(v1.OperationPut)
		require.True(t, result.Requeue)
		require.Equal(t, v1.ProvisioningStateUpdating, result.ProvisioningState())
		require.NotEmpty(t, f.record().Properties.RequestToken)
	})

	t.Run("the worker stops", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		f.cc.Errors["GetResourceRequestStatus"] = errors.New("canceled")
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		c, err := NewAwaitController(ctrl.Options{DatabaseClient: f.db}, ucp_aws.Clients{CloudControl: f.cc}, v1.OperationPut)
		require.NoError(t, err)
		_, err = c.Run(ctx, &ctrl.Request{ResourceID: vpcID, OperationID: f.op})
		require.Error(t, err)
	})

	t.Run("no record", func(t *testing.T) {
		f := newFixture(t)
		require.Nil(t, f.run(v1.OperationDelete).Error)
		require.Contains(t, f.run(v1.OperationPut).Error.Message, "no record")
	})

	t.Run("invalid resource ID", func(t *testing.T) {
		f := newFixture(t)
		c, err := NewAwaitController(ctrl.Options{DatabaseClient: f.db}, ucp_aws.Clients{CloudControl: f.cc}, v1.OperationPut)
		require.NoError(t, err)
		result, err := c.Run(t.Context(), &ctrl.Request{ResourceID: "not an id"})
		require.NoError(t, err)
		require.Equal(t, v1.CodeInvalid, result.Error.Code)
	})

	t.Run("reading the record fails, so the operation is retried", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		result := f.runWith(t.Context(), v1.OperationPut, brokenDB{Client: f.db, failGet: true})
		require.True(t, result.Requeue)
	})

	t.Run("saving the result fails, so the operation is retried", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		result := f.runWith(t.Context(), v1.OperationPut, brokenDB{Client: f.db, failSave: true})
		require.True(t, result.Requeue)

		// The retry saves the result.
		result = f.run(v1.OperationPut)
		require.Nil(t, result.Error)
		require.Equal(t, map[string]any{"CidrBlock": "10.0.0.0/16"}, f.record().Properties.LastApplied)
	})

	t.Run("saving a failure fails, so the operation is retried", func(t *testing.T) {
		f := newFixture(t)
		f.cc.FailNext = "InternalFailure"
		f.accept(records.OperationUpdate)
		result := f.runWith(t.Context(), v1.OperationPut, brokenDB{Client: f.db, failSave: true})
		require.True(t, result.Requeue)
	})
}

func Test_Await_Delete(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationUpdate)
	f.run(v1.OperationPut)
	out, err := f.cc.DeleteResource(t.Context(), &cloudcontrol.DeleteResourceInput{TypeName: aws.String(vpcType), Identifier: aws.String("id-1")})
	require.NoError(t, err)
	f.saveWith(func(rec *datamodel.AWSResource) {
		rec.Properties.RequestToken = aws.ToString(out.ProgressEvent.RequestToken)
		rec.SetProvisioningState(v1.ProvisioningStateDeleting)
	})
	f.op = uuid.MustParse(aws.ToString(out.ProgressEvent.RequestToken))
	require.Nil(t, f.run(v1.OperationDelete).Error)
	require.Nil(t, f.record())
}

// useClock makes records.Now advance by a second on every read, so every save of a record changes its content and its
// ETag. Otherwise two saves in the same clock tick write the same content and do not conflict.
func useClock(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	records.Now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Second)
		return now
	}
	t.Cleanup(func() { records.Now = time.Now })
}

func Test_Update_RetriesConcurrentSaves(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationUpdate)
	n := 0
	err := update(t.Context(), f.store, f.id, f.op, func(rec *datamodel.AWSResource) bool {
		n++
		if n == 1 {
			// Another writer, such as the heartbeat, saves the record first.
			require.NoError(t, f.store.Touch(t.Context(), f.id))
		}
		rec.Properties.Owner = "changed"
		return false
	})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Equal(t, "changed", f.record().Properties.Owner)

	err = update(t.Context(), f.store, f.id, f.op, func(rec *datamodel.AWSResource) bool {
		require.NoError(t, f.store.Touch(t.Context(), f.id))
		return false
	})
	require.ErrorContains(t, err, "kept changing")
}

func Test_Heartbeat(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationUpdate)
	before := records.LastModified(f.record())
	stop := heartbeat(t.Context(), f.store, f.id, time.Millisecond)
	require.Eventually(t, func() bool { return records.LastModified(f.record()).After(before) }, time.Second, time.Millisecond)
	stop()

	// A failed save is logged and the heartbeat keeps going.
	gets := &atomic.Int32{}
	stop = heartbeat(t.Context(), records.NewStore(brokenDB{Client: f.db, failGet: true, gets: gets}), f.id, time.Millisecond)
	require.Eventually(t, func() bool { return gets.Load() >= 2 }, time.Second, time.Millisecond)
	stop()
}

func Test_NewAwaitController_RequiresCloudControl(t *testing.T) {
	_, err := NewAwaitController(ctrl.Options{}, ucp_aws.Clients{}, v1.OperationPut)
	require.Error(t, err)
}

func Test_Register(t *testing.T) {
	registry := worker.NewControllerRegistry()
	require.NoError(t, Register(registry, ucp_aws.Clients{CloudControl: awsfake.NewCloudControl()}, ctrl.Options{DatabaseClient: inmemory.NewClient()}))
	for _, method := range []v1.OperationMethod{v1.OperationPut, v1.OperationDelete} {
		c, err := registry.Get(v1.OperationType{Type: records.OperationType, Method: method})
		require.NoError(t, err)
		require.IsType(t, &AwaitController{}, c)
	}
}

// Test_Await_Redelivery covers a message the worker gets again after it already finished the operation, for example
// because UCP stopped after saving the record and before saving the operation status.
func Test_Await_Redelivery(t *testing.T) {
	t.Run("after success", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationCreate)
		require.Nil(t, f.run(v1.OperationPut).Error)
		before := f.record()

		result := f.run(v1.OperationPut)
		require.Nil(t, result.Error)
		require.Equal(t, v1.ProvisioningStateSucceeded, result.ProvisioningState())
		require.Equal(t, before.Properties, f.record().Properties)
	})

	t.Run("after a failed update", func(t *testing.T) {
		f := newFixture(t)
		f.cc.FailNext = "InvalidRequest"
		f.accept(records.OperationUpdate)
		first := f.run(v1.OperationPut)
		require.Equal(t, v1.CodeInvalid, first.Error.Code)

		again := f.run(v1.OperationPut)
		require.Equal(t, first.Error, again.Error)
		require.Equal(t, v1.ProvisioningStateFailed, again.ProvisioningState())
	})

	t.Run("an old delete after the alias was created again", func(t *testing.T) {
		f := newFixture(t)
		f.accept(records.OperationUpdate)
		oldDelete := uuid.New()
		// The delete finished and removed the record, then a new deploy created the alias again.
		f.accept(records.OperationCreate)
		creating := f.record()

		f.op = oldDelete
		// Polling the new create's request would fail the test.
		f.cc.Errors["GetResourceRequestStatus"] = errors.New("must not poll")
		result := f.run(v1.OperationDelete)
		require.Equal(t, v1.ProvisioningStateCanceled, result.ProvisioningState())
		require.Contains(t, result.Error.Message, "newer operation")
		require.Equal(t, creating.Properties, f.record().Properties, "the old delete must not poll or finish the new create")
	})
}

// Test_Update_Superseded checks that finishing an operation changes nothing once the record shows a newer one.
func Test_Update_Superseded(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationUpdate)
	old := f.op
	f.accept(records.OperationUpdate)
	err := update(t.Context(), f.store, f.id, old, func(rec *datamodel.AWSResource) bool { return true })
	require.ErrorIs(t, err, errSuperseded)
	require.NotNil(t, f.record())
}

// Test_Update_DeleteRetriesConcurrentSaves checks that the record is removed only if nothing changed it since it was
// read.
func Test_Update_DeleteRetriesConcurrentSaves(t *testing.T) {
	f := newFixture(t)
	f.accept(records.OperationUpdate)
	n := 0
	err := update(t.Context(), f.store, f.id, f.op, func(rec *datamodel.AWSResource) bool {
		n++
		if n == 1 {
			require.NoError(t, f.store.Touch(t.Context(), f.id))
		}
		return true
	})
	require.NoError(t, err)
	require.Equal(t, 2, n)
	require.Nil(t, f.record())
}

func (f *fixture) saveWith(change func(rec *datamodel.AWSResource)) {
	f.t.Helper()
	rec, etag, err := f.store.Get(f.t.Context(), f.id)
	require.NoError(f.t, err)
	change(rec)
	_, err = f.store.Save(f.t.Context(), rec, etag)
	require.NoError(f.t, err)
}

type brokenDB struct {
	database.Client
	failGet  bool
	failSave bool
	gets     *atomic.Int32
}

func (b brokenDB) Get(ctx context.Context, id string, options ...database.GetOptions) (*database.Object, error) {
	if b.gets != nil {
		b.gets.Add(1)
	}
	if b.failGet {
		return nil, errors.New("database is down")
	}
	return b.Client.Get(ctx, id, options...)
}

func (b brokenDB) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	if b.failSave {
		return errors.New("database is down")
	}
	return b.Client.Save(ctx, obj, options...)
}
