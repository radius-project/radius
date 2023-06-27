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

package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/project-radius/radius/pkg/ucp/queue/inmemory"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

var (
	testResourceType    = "Applications.Core/environments"
	testOperationStatus = &manager.Status{
		AsyncOperationStatus: v1.AsyncOperationStatus{
			ID:        uuid.NewString(),
			Name:      "operation-status",
			Status:    v1.ProvisioningStateUpdating,
			StartTime: time.Now().UTC(),
		},
		LinkedResourceID: uuid.New().String(),
		Location:         "test-location",
		HomeTenantID:     "test-home-tenant-id",
		ClientObjectID:   "test-client-object-id",
	}
	defaultTestLockTime = defaultMinMessageLockDuration * 2
)

type testAsyncController struct {
	ctrl.BaseController
	fn func(ctx context.Context) (ctrl.Result, error)
}

// # Function Explanation
// 
//	The testAsyncController's Run function provides an asynchronous controller that can be used to execute a given function 
//	with a context, and returns a ctrl.Result and an error if one occurs.
func (c *testAsyncController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	if c.fn != nil {
		return c.fn(ctx)
	}
	return ctrl.Result{}, nil
}

type testContext struct {
	ctx    context.Context
	mockSC *store.MockStorageClient
	mockSM *manager.MockStatusManager
	mockSP *dataprovider.MockDataStorageProvider

	testQueue *inmemory.Client
	internalQ *inmemory.InmemQueue
}

// newTestResourceObject returns new store.Object to prevent datarace when updateResourceState accesses map[string]any{} concurrently.
func newTestResourceObject() *store.Object {
	return &store.Object{
		Data: map[string]any{
			"name":              "env0",
			"provisioningState": "Accepted",
			"properties":        map[string]any{},
		},
	}
}

func (c *testContext) drainQueueOrAssert(t *testing.T) {
	startAt := time.Now()
	for c.internalQ.Len() > 0 {
		if time.Until(startAt) > 20*time.Second {
			require.Fail(t, "failed to drain queue by worker")
		}
		time.Sleep(time.Millisecond)
	}
}

func (c *testContext) cancellable(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == time.Duration(0) {
		return context.WithCancel(c.ctx)
	} else {
		return context.WithTimeout(c.ctx, timeout)
	}
}

func newTestContext(t *testing.T, lockTime time.Duration) (*testContext, *gomock.Controller) {
	mctrl := gomock.NewController(t)
	inmemQ := inmemory.NewInMemQueue(lockTime)
	return &testContext{
		ctx:       context.Background(),
		mockSC:    store.NewMockStorageClient(mctrl),
		mockSM:    manager.NewMockStatusManager(mctrl),
		mockSP:    dataprovider.NewMockDataStorageProvider(mctrl),
		internalQ: inmemQ,
		testQueue: inmemory.New(inmemQ),
	}, mctrl
}

func genTestMessage(opID uuid.UUID, opTimeout time.Duration) *queue.Message {
	testMessage := queue.NewMessage(&ctrl.Request{
		OperationID:   opID,
		OperationType: "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
		ResourceID: fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/%s",
			uuid.NewString()),
		CorrelationID:    uuid.NewString(),
		OperationTimeout: &opTimeout,
	})

	testMessage.Metadata = queue.Metadata{
		DequeueCount:  0,
		NextVisibleAt: time.Now(),
	}

	return testMessage
}

func TestStart_UnknownOperation(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := New(Options{}, nil, tCtx.testQueue, registry)

	tCtx.mockSP.EXPECT().
		GetStorageClient(gomock.Any(), gomock.Any()).
		Return(tCtx.mockSC, nil).
		Times(1)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	called := false
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			called = true
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		testResourceType, "UNDEFINED",
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, opts)

	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	// Queue async operation.
	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop
	cancel()
	<-done

	require.Equal(t, 1, testMessage.DequeueCount)
	require.False(t, called)
}

func TestStart_MaxDequeueCount(t *testing.T) {
	tCtx, mctrl := newTestContext(t, 1*time.Minute)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(v1.ProvisioningStateFailed), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(tCtx.mockSC), nil).Times(1)

	expectedDequeueCount := 2

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := New(Options{MaxOperationRetryCount: expectedDequeueCount}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(0)
	err := registry.Register(
		ctx,
		testResourceType, v1.OperationPut,
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, ctrl.Options{
			DataProvider: tCtx.mockSP,
		})
	require.NoError(t, err)

	// Queue async operation.
	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	testMessage.DequeueCount = expectedDequeueCount + 1

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop
	cancel()
	<-done

	require.Equal(t, expectedDequeueCount+2, testMessage.DequeueCount)
}

func TestStart_MaxConcurrency(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testOperationStatus, nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(tCtx.mockSC), nil).AnyTimes()

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	// register test controller.
	cnt := atomic.NewInt32(0)
	maxConcurrency := atomic.NewInt32(0)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			cnt.Inc()
			if maxConcurrency.Load() < cnt.Load() {
				maxConcurrency.Store(cnt.Load())
			}
			time.Sleep(100 * time.Millisecond)
			cnt.Dec()
			return ctrl.Result{}, nil
		},
	}
	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		testResourceType,
		v1.OperationPut,
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, opts)
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	testMessageCnt := 10
	testMessages := []*queue.Message{}
	// queue asyncoperation messages.
	for i := 0; i < testMessageCnt; i++ {
		testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
		testMessages = append(testMessages, testMessage)
		err = tCtx.testQueue.Enqueue(ctx, testMessage)
		require.NoError(t, err)
	}

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop.
	cancel()
	<-done

	for i := 0; i < testMessageCnt; i++ {
		require.Equal(t, 1, testMessages[i].DequeueCount)
	}
	require.Equal(t, int32(defaultMaxOperationConcurrency), maxConcurrency.Load())
}

func TestStart_RunOperation(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testOperationStatus, nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(tCtx.mockSC), nil).AnyTimes()

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	called := make(chan bool, 1)
	var opCtx context.Context
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			// operation context will be cancelled after this function is called
			opCtx = ctx
			called <- true
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		testResourceType, v1.OperationPut,
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, opts)
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	// Queue async operation.
	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	<-called

	// Wait until operation context is done.
	<-opCtx.Done()

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop
	cancel()
	<-done

	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
	require.Equal(t, 1, testMessage.DequeueCount)
}

func TestRunOperation_Successfully(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
	}

	require.Equal(t, 1, tCtx.internalQ.Len())

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx)
	require.NoError(t, err)
	worker.runOperation(context.Background(), msg, testCtrl)

	// Ensure that message is finished.
	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
}

func TestRunOperation_ExtendMessageLock(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			// Sleep for longer than minimum message lock time to call client.ExtendMessage
			time.Sleep(defaultMinMessageLockDuration * 2)
			return ctrl.Result{}, nil
		},
	}

	require.Equal(t, 1, tCtx.internalQ.Len())

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx)
	old := msg.NextVisibleAt
	require.NoError(t, err)

	worker.runOperation(context.Background(), msg, testCtrl)

	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
	require.Greater(t, msg.NextVisibleAt.UnixNano(), old.UnixNano(), "message lock is extended")
}

func TestRunOperation_CancelContext(t *testing.T) {
	tCtx, _ := newTestContext(t, defaultTestLockTime)

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	worker := New(Options{}, nil, tCtx.testQueue, nil)

	opts := ctrl.Options{
		StorageClient: nil,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return nil
		},
	}

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			<-ctx.Done()
			close(done)
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(10 * time.Millisecond)
	require.Equal(t, 1, tCtx.internalQ.Len())

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx)
	require.NoError(t, err)

	worker.runOperation(ctx, msg, testCtrl)

	<-done
	cancel()

	require.Equal(t, 1, tCtx.internalQ.Len(), "ensure that message is not finished")
}

func TestRunOperation_Timeout(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...store.GetOptions) (*store.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ resources.ID, _ uuid.UUID, state v1.ProvisioningState, _ *time.Time, opError *v1.ErrorDetails) error {
			if state == v1.ProvisioningStateCanceled && strings.HasPrefix(opError.Message, "Operation (APPLICATIONS.CORE/ENVIRONMENTS|PUT) has timed out because it was processing longer than") &&
				strings.HasPrefix(opError.Target, "/subscriptions/00000000-0000-0000-0000-000000000000") {
				return nil
			}
			return errors.New("!!! failed to update status !!!")
		}).Times(1)

	testMessage := genTestMessage(uuid.New(), 10*time.Millisecond)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			<-ctx.Done()
			close(done)
			return ctrl.Result{}, nil
		},
	}

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx)
	require.NoError(t, err)
	worker.runOperation(context.Background(), msg, testCtrl)
	<-done

	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
}

func TestRunOperation_PanicController(t *testing.T) {
	tCtx, _ := newTestContext(t, defaultTestLockTime)

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	worker := New(Options{}, nil, tCtx.testQueue, nil)

	opts := ctrl.Options{
		StorageClient: tCtx.mockSC,
		DataProvider:  tCtx.mockSP,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return nil
		},
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			panic("!!! don't panic !!!")
		},
	}

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx)
	require.NoError(t, err)

	require.NotPanics(t, func() {
		worker.runOperation(tCtx.ctx, msg, testCtrl)
	})

	require.Equal(t, 1, tCtx.internalQ.Len(), "ensure that message is not finished")
}
