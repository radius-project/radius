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

func getTestMessage() (*queue.Message, *atomic.Int32, *atomic.Int32) {
	finished := atomic.NewInt32(0)
	extended := atomic.NewInt32(0)
	testMessage := &queue.Message{
		Metadata: queue.Metadata{
			DequeueCount:  0,
			NextVisibleAt: time.Now().Add(time.Duration(120) * time.Second),
		},
		Data: &asyncoperation.Request{
			OperationID:      uuid.New(),
			OperationType:    "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
			ResourceID:       "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/env0",
			CorrelationID:    uuid.NewString(),
			OperationTimeout: &asyncoperation.DefaultAsyncOperationTimeout,
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

func getTestMocks(mctrl *gomock.Controller) (*store.MockStorageClient, *asyncoperation.MockStatusManager) {
	mockSC := store.NewMockStorageClient(mctrl)
	mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		&store.Object{
			Data: map[string]interface{}{
				"name": "env0",
				"properties": map[string]interface{}{
					"provisioningState": "Updating",
				},
			},
		}, nil).AnyTimes()
	mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockOpManager := asyncoperation.NewMockStatusManager(mctrl)
	mockOpManager.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	return mockSC, mockOpManager
}

func TestStart_UnknownOperation(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil)

	inmemQ := inmemory.NewInMemQueue(5 * time.Minute)
	testQueue := inmemory.NewClient(inmemQ)
	registry := NewControllerRegistry(mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, testQueue, registry)
	testMessage, _, _ := getTestMessage()

	ctx, cancel := context.WithCancel(context.Background())
	err := registry.Register(ctx, asyncoperation.OperationType{Type: "Applications.Core/environments", Method: "UNDEFINED"}, func(s store.StorageClient) (asyncoperation.Controller, error) {
		return nil, nil
	})
	require.NoError(t, err)

	done := make(chan bool, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		done <- true
	}()

	// Queue async operation.
	err = testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)

	for inmemQ.Len() > 0 {
		// Wait until queue is empty
	}

	// Cancelling worker loop
	cancel()
	<-done
}

func TestStart_MaxDequeueCount(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(nil, nil)

	inmemQ := inmemory.NewInMemQueue(5 * time.Minute)
	testQueue := inmemory.NewClient(inmemQ)
	registry := NewControllerRegistry(mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, testQueue, registry)
	testMessage, _, _ := getTestMessage()

	ctx, cancel := context.WithCancel(context.Background())
	err := registry.Register(ctx, asyncoperation.OperationType{Type: "Applications.Core/environments", Method: "PUT"},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return nil, nil
		})
	require.NoError(t, err)

	done := make(chan bool, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		done <- true
	}()

	// Queue async operation.
	err = testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	testMessage.DequeueCount = MaxDequeueCount + 1

	for inmemQ.Len() > 0 {
		// Wait until queue is empty.
	}

	// Cancelling worker loop
	cancel()
	<-done
}

func TestStart_MaxConcurrency(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSC, mockOpManager := getTestMocks(mctrl)

	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)
	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil).AnyTimes()

	inmemQ := inmemory.NewInMemQueue(5 * time.Minute)
	testQueue := inmemory.NewClient(inmemQ)
	registry := NewControllerRegistry(mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, mockOpManager, testQueue, registry)

	cnt := atomic.NewInt32(0)
	maxConcurrency := atomic.NewInt32(0)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(mockSC),
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
	ctx, cancel := context.WithCancel(context.Background())
	err := registry.Register(ctx, asyncoperation.OperationType{Type: "Applications.Core/environments", Method: "PUT"},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return testCtrl, nil
		})
	require.NoError(t, err)

	done := make(chan bool, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		done <- true
	}()

	// Queue async operation.
	for i := 0; i < 10; i++ {
		testMessage, _, _ := getTestMessage()
		err = testQueue.Enqueue(ctx, testMessage)
		require.NoError(t, err)
	}

	for inmemQ.Len() > 0 {
		// Wait until queue is empty.
	}

	// Cancelling worker loop
	cancel()
	<-done

	require.Equal(t, int32(MaxOperationConcurrency), maxConcurrency.Load())
}

func TestStart_RunOperation(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSC, mockOpManager := getTestMocks(mctrl)
	mockSP := dataprovider.NewMockDataStorageProvider(mctrl)

	mockSP.EXPECT().GetStorageClient(gomock.Any(), gomock.Any()).Return(store.StorageClient(mockSC), nil)

	testQueue := inmemory.NewClient(nil)
	registry := NewControllerRegistry(mockSP)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, mockOpManager, testQueue, registry)
	testMessage, _, _ := getTestMessage()

	called := make(chan bool, 1)
	var opCtx context.Context
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			opCtx = ctx
			called <- true
			return asyncoperation.Result{}, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := registry.Register(ctx, asyncoperation.OperationType{Type: "Applications.Core/environments", Method: "PUT"},
		func(s store.StorageClient) (asyncoperation.Controller, error) {
			return testCtrl, nil
		})
	require.NoError(t, err)

	done := make(chan bool, 1)
	go func() {
		err = worker.Start(ctx)
		require.NoError(t, err)
		done <- true
	}()

	// Queue async operation.
	err = testQueue.Enqueue(ctx, testMessage)
	require.NoError(t, err)
	<-called

	// Wait until operation context is done.
	<-opCtx.Done()

	// Cancelling worker loop
	cancel()
	<-done
}

func TestRunOperation_Successfully(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSC, mockOpManager := getTestMocks(mctrl)
	testQueue := inmemory.NewClient(nil)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, mockOpManager, testQueue, nil)

	testMessage, finished, extended := getTestMessage()

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(mockSC),
	}

	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}

func TestRunOperation_ExtendMessageLock(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSC, mockOpManager := getTestMocks(mctrl)
	testQueue := inmemory.NewClient(nil)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, mockOpManager, testQueue, nil)

	testMessage, finished, extended := getTestMessage()

	testMessage.NextVisibleAt = time.Now().Add(5 * time.Second)

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			time.Sleep(6 * time.Second)
			return asyncoperation.Result{}, nil
		},
	}

	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(1), extended.Load())
}

func TestRunOperation_CancelContext(t *testing.T) {
	testQueue := inmemory.NewClient(nil)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, testQueue, nil)

	testMessage, finished, extended := getTestMessage()

	done := make(chan bool, 1)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			<-ctx.Done()
			done <- true
			return asyncoperation.Result{}, nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	worker.runOperation(ctx, testMessage, testCtrl)

	<-done

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())

	cancel()
}

func TestRunOperation_Timeout(t *testing.T) {
	mctrl := gomock.NewController(t)
	defer mctrl.Finish()

	mockSC, mockOpManager := getTestMocks(mctrl)
	testQueue := inmemory.NewClient(nil)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, mockOpManager, testQueue, nil)

	testMessage, finished, extended := getTestMessage()
	req := testMessage.Data.(*asyncoperation.Request)
	opTimeout := 10 * time.Millisecond
	req.OperationTimeout = &opTimeout

	done := make(chan bool, 1)
	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(mockSC),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			<-ctx.Done()
			done <- true
			return asyncoperation.Result{}, nil
		},
	}

	worker.runOperation(context.Background(), testMessage, testCtrl)
	<-done

	require.Equal(t, int32(1), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}

func TestRunOperation_PanicController(t *testing.T) {
	testQueue := inmemory.NewClient(nil)
	worker := NewAsyncRequestProcessWorker(hostoptions.HostOptions{}, nil, testQueue, nil)

	testMessage, finished, extended := getTestMessage()

	testCtrl := &testAsyncController{
		BaseController: asyncoperation.NewBaseAsyncController(nil),
		fn: func(ctx context.Context) (asyncoperation.Result, error) {
			panic("don't panic")
		},
	}

	worker.runOperation(context.Background(), testMessage, testCtrl)

	require.Equal(t, int32(0), finished.Load())
	require.Equal(t, int32(0), extended.Load())
}
