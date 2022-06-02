// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/queue/inmemory"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

var (
	testResourceType   = "Applications.Core/environments"
	testResourceObject = &store.Object{
		Data: map[string]interface{}{
			"name": "env0",
			"properties": map[string]interface{}{
				"provisioningState": "Updating",
			},
		},
	}
)

type testAsyncController struct {
	asyncoperation.BaseController
	fn func(ctx context.Context) (asyncoperation.Result, error)
}

func (ctrl *testAsyncController) Run(ctx context.Context, request *asyncoperation.Request) (asyncoperation.Result, error) {
	if ctrl.fn != nil {
		return ctrl.fn(ctx)
	}
	return asyncoperation.Result{}, nil
}

type testContext struct {
	ctx    context.Context
	mockSC *store.MockStorageClient
	mockSM *asyncoperation.MockStatusManager
	mockSP *dataprovider.MockDataStorageProvider

	testQueue *inmemory.Client
	internalQ *inmemory.InmemQueue
}

func (c *testContext) drainQueue() {
	for c.internalQ.Len() > 0 {
		// Wait until queue is empty
	}
}

func (c *testContext) cancellable(timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout == time.Duration(0) {
		return context.WithCancel(c.ctx)
	} else {
		return context.WithTimeout(c.ctx, timeout)
	}
}

func newTestContext(t *testing.T) (*testContext, *gomock.Controller) {
	mctrl := gomock.NewController(t)
	inmemQ := inmemory.NewInMemQueue(5 * time.Minute)
	return &testContext{
		ctx:       context.Background(),
		mockSC:    store.NewMockStorageClient(mctrl),
		mockSM:    asyncoperation.NewMockStatusManager(mctrl),
		mockSP:    dataprovider.NewMockDataStorageProvider(mctrl),
		internalQ: inmemQ,
		testQueue: inmemory.NewClient(inmemQ),
	}, mctrl
}

func genTestMessage(opID uuid.UUID, opTimeout time.Duration) (*queue.Message, *atomic.Int32, *atomic.Int32) {
	finished := atomic.NewInt32(0)
	extended := atomic.NewInt32(0)

	testMessage := &queue.Message{
		Metadata: queue.Metadata{
			DequeueCount:  0,
			NextVisibleAt: time.Now().Add(time.Duration(120) * time.Second),
		},
		Data: &asyncoperation.Request{
			OperationID:      opID,
			OperationType:    "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &opTimeout,
		},
	}
	testMessage.WithExtend(func() error {
		extended.Inc()
		return nil
	})
	testMessage.WithFinish(func(err error) error {
		finished.Inc()
		return err
	})

	return testMessage, finished, extended
}

func TestStart_UnknownOperation(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil)

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, tCtx.testQueue, registry)

	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		asyncoperation.OperationType{Type: testResourceType, Method: "UNDEFINED"},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return nil, nil
		})

	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	// Queue async operation.
	testMessage, _, _ := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)

	tCtx.drainQueue()

	// Cancelling worker loop
	cancel()
	<-done
}

func TestStart_MaxDequeueCount(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil)

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, tCtx.testQueue, registry)

	ctx, cancel := tCtx.cancellable(0)
	err := registry.Register(
		ctx,
		asyncoperation.OperationType{Type: testResourceType, Method: asyncoperation.OperationPut},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return nil, nil
		})
	require.NoError(t, err)

	// Queue async operation.
	testMessage, _, _ := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	testMessage.DequeueCount = MaxDequeueCount

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	tCtx.drainQueue()

	// Cancelling worker loop
	cancel()
	<-done
}

func TestStart_MaxConcurrency(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(tCtx.mockSC), nil).AnyTimes()

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, tCtx.mockSM, tCtx.testQueue, registry)

	// register test controller.
	cnt := atomic.NewInt32(0)
	maxConcurrency := atomic.NewInt32(0)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			cnt.Inc()
			if maxConcurrency.Load() < cnt.Load() {
				maxConcurrency.Store(cnt.Load())
			}
			time.Sleep(100 * time.Millisecond)
			cnt.Dec()
			return asyncoperation.Result{}, nil
		},
	}
	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		asyncoperation.OperationType{Type: testResourceType, Method: asyncoperation.OperationPut},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return testCtrl, nil
		})
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	// queue asyncoperation messages.
	for i := 0; i < 10; i++ {
		testMessage, _, _ := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
		err = tCtx.testQueue.Enqueue(ctx, testMessage)
		require.NoError(t, err)
	}

	tCtx.drainQueue()

	// Cancelling worker loop.
	cancel()
	<-done

	require.Equal(t, int32(MaxOperationConcurrency), maxConcurrency.Load())
}

func TestStart_RunOperation(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(tCtx.mockSC), nil).AnyTimes()

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, tCtx.mockSM, tCtx.testQueue, registry)

	called := make(chan bool, 1)
	var opCtx context.Context
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			// operation context will be cancelled after this function is called
			opCtx = ctx
			called <- true
			return asyncoperation.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		asyncoperation.OperationType{Type: testResourceType, Method: asyncoperation.OperationPut},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return testCtrl, nil
		})
	require.NoError(t, err)

	done := make(chan struct{}, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		close(done)
	}()

	// Queue async operation.
	testMessage, _, _ := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	<-called

	// Wait until operation context is done.
	<-opCtx.Done()

	// Cancelling worker loop
	cancel()
	<-done
}

func TestRunOperation_Successfully(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, tCtx.mockSM, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(tCtx.mockSC),
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}

func TestRunOperation_ExtendMessageLock(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, tCtx.mockSM, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			time.Sleep(minMessageLockDuration + time.Duration(1)*time.Second)
			return asyncoperation.Result{}, nil
		},
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	testMessage.NextVisibleAt = time.Now().Add(minMessageLockDuration)

	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(1), extended.Load())
}

func TestRunOperation_CancelContext(t *testing.T) {
	tCtx, _ := newTestContext(t)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, tCtx.testQueue, nil)

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			<-ctx.Done()
			close(done)
			return asyncoperation.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(10 * time.Millisecond)
	testMessage, finished, extended := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	worker.runOperation(ctx, testMessage, testCtrl)

	<-done

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())

	cancel()
}

func TestRunOperation_Timeout(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, tCtx.mockSM, tCtx.testQueue, nil)

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			<-ctx.Done()
			close(done)
			return asyncoperation.Result{}, nil
		},
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), 10*time.Millisecond)
	worker.runOperation(context.Background(), testMessage, testCtrl)
	<-done

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}

func TestRunOperation_PanicController(t *testing.T) {
	tCtx, _ := newTestContext(t)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			panic("!!! don't panic !!!")
		},
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), asyncoperation.DefaultAsyncOperationTimeout)
	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}
