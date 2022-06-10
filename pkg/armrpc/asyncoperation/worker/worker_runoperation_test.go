// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/queue/inmemory"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
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
	ctrl.BaseController
	fn func(ctx context.Context) (ctrl.Result, error)
}

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

func newTestContext(t *testing.T) (*testContext, *gomock.Controller) {
	mctrl := gomock.NewController(t)
	inmemQ := inmemory.NewInMemQueue(5 * time.Minute)
	return &testContext{
		ctx:       context.Background(),
		mockSC:    store.NewMockStorageClient(mctrl),
		mockSM:    manager.NewMockStatusManager(mctrl),
		mockSP:    dataprovider.NewMockDataStorageProvider(mctrl),
		internalQ: inmemQ,
		testQueue: inmemory.New(inmemQ),
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
		Data: &ctrl.Request{
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
	worker := New(Options{}, nil, tCtx.testQueue, registry)

	called := false
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			called = true
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
		ctx,
		v1.OperationType{Type: testResourceType, Method: "UNDEFINED"},
		func(s store.StorageClient) (ctrl.Controller, error) {
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
	testMessage, _, _ := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
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
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	tCtx.mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil)

	registry := NewControllerRegistry(tCtx.mockSP)
	worker := New(Options{}, nil, tCtx.testQueue, registry)

	ctx, cancel := tCtx.cancellable(0)
	err := registry.Register(
		ctx,
		v1.OperationType{Type: testResourceType, Method: v1.OperationPut},
		func(s store.StorageClient) (ctrl.Controller, error) {
			return nil, nil
		})
	require.NoError(t, err)

	// Queue async operation.
	testMessage, _, _ := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	testMessage.DequeueCount = MaxDequeueCount

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

	require.Equal(t, MaxDequeueCount+1, testMessage.DequeueCount)
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
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, registry)

	// register test controller.
	cnt := atomic.NewInt32(0)
	maxConcurrency := atomic.NewInt32(0)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
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
		v1.OperationType{Type: testResourceType, Method: v1.OperationPut},
		func(s store.StorageClient) (ctrl.Controller, error) {
			return testCtrl, nil
		})
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
		testMessage, _, _ := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
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
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, registry)

	called := make(chan bool, 1)
	var opCtx context.Context
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
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
		v1.OperationType{Type: testResourceType, Method: v1.OperationPut},
		func(s store.StorageClient) (ctrl.Controller, error) {
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
	testMessage, _, _ := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err = tCtx.testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	<-called

	// Wait until operation context is done.
	<-opCtx.Done()

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop
	cancel()
	<-done

	require.Equal(t, 0, tCtx.internalQ.Len())
	require.Equal(t, 1, testMessage.DequeueCount)
}

func TestRunOperation_Successfully(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
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

	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			time.Sleep(minMessageLockDuration + time.Duration(1)*time.Second)
			return ctrl.Result{}, nil
		},
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	testMessage.NextVisibleAt = time.Now().Add(minMessageLockDuration)

	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(1), extended.Load())
}

func TestRunOperation_CancelContext(t *testing.T) {
	tCtx, _ := newTestContext(t)
	worker := New(Options{}, nil, tCtx.testQueue, nil)

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			<-ctx.Done()
			close(done)
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(10 * time.Millisecond)
	testMessage, finished, extended := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	worker.runOperation(ctx, testMessage, testCtrl)

	<-done
	cancel()

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}

func TestRunOperation_Timeout(t *testing.T) {
	tCtx, mctrl := newTestContext(t)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testResourceObject, nil).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	done := make(chan struct{}, 1)
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(tCtx.mockSC),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			<-ctx.Done()
			close(done)
			return ctrl.Result{}, nil
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
	worker := New(Options{}, nil, tCtx.testQueue, nil)

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			panic("!!! don't panic !!!")
		},
	}

	testMessage, finished, extended := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	require.NotPanics(t, func() {
		worker.runOperation(tCtx.ctx, testMessage, testCtrl)
	})

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}
