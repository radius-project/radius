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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"uuid"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/components/database"
	inmemorystore "github.com/radius-project/radius/pkg/components/database/inmemory"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/components/queue/inmemory"
	"github.com/radius-project/radius/pkg/corerp/backend/deployment"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const (
	// defaultTestDequeueInterval is the default duration for the dequeue interval for inmemory test queue
	defaultTestDequeueInterval = time.Duration(5) * time.Millisecond
)

var (
	testResourceType    = "Applications.Core/environments"
	testOperationStatus = &manager.Status{
		ID:               uuid.New().String(),
		Name:             "operation-status",
		Status:           v1.ProvisioningStateUpdating,
		StartTime:        time.Now().UTC(),
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

func (c *testAsyncController) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	if c.fn != nil {
		return c.fn(ctx)
	}
	return ctrl.Result{}, nil
}

type testContext struct {
	ctx    context.Context
	mockSC *database.MockClient
	mockSM *manager.MockStatusManager

	testQueue *inmemory.Client
	internalQ *inmemory.InmemQueue
}

// newTestResourceObject returns new store.Object to prevent datarace when updateResourceState accesses map[string]any{} concurrently.
func newTestResourceObject() *database.Object {
	return &database.Object{
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
	databaseClient := database.NewMockClient(mctrl)
	return &testContext{
		ctx:       t.Context(),
		mockSC:    databaseClient,
		mockSM:    manager.NewMockStatusManager(mctrl),
		internalQ: inmemQ,
		testQueue: inmemory.New(inmemQ),
	}, mctrl
}

func genTestMessage(opID uuid.UUID, opTimeout time.Duration) *queue.Message {
	testMessage := queue.NewMessage(&ctrl.Request{
		OperationID:   opID,
		OperationType: "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
		ResourceID: fmt.Sprintf("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/Applications.Core/environments/%s",
			uuid.New().String()),
		CorrelationID:    uuid.New().String(),
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

	registry := NewControllerRegistry()
	worker := New(Options{DequeueIntervalDuration: defaultTestDequeueInterval}, nil, tCtx.testQueue, registry)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
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
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	// The operation is not yet terminal, so the max-retry path completes it with the generic message.
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&manager.Status{
			Status: v1.ProvisioningStateUpdating,
		}, nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(v1.ProvisioningStateFailed), gomock.Any(), gomock.Any()).Return(nil).Times(1)

	expectedDequeueCount := 2

	registry := NewControllerRegistry()
	worker := New(Options{MaxOperationRetryCount: expectedDequeueCount, DequeueIntervalDuration: defaultTestDequeueInterval}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(0)
	err := registry.Register(
		testResourceType, v1.OperationPut,
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, ctrl.Options{
			DatabaseClient: tCtx.mockSC,
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

// TestStart_MaxDequeueCount_AlreadyTerminal verifies that when the retry count is exceeded but the
// operation was already completed on a prior attempt (terminal status), the worker just finishes the
// message and does NOT overwrite the recorded status with a generic "exceeded max retry count" error.
func TestStart_MaxDequeueCount_AlreadyTerminal(t *testing.T) {
	tCtx, mctrl := newTestContext(t, 1*time.Minute)
	defer mctrl.Finish()

	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	// The operation is already terminal (Failed) from a prior attempt.
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&manager.Status{
			Status: v1.ProvisioningStateFailed,
		}, nil).AnyTimes()
	// No Update must happen: the recorded terminal status must not be overwritten.
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	expectedDequeueCount := 2

	registry := NewControllerRegistry()
	worker := New(Options{MaxOperationRetryCount: expectedDequeueCount, DequeueIntervalDuration: defaultTestDequeueInterval}, tCtx.mockSM, tCtx.testQueue, registry)

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(ctrl.Options{DatabaseClient: tCtx.mockSC}),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			return ctrl.Result{}, nil
		},
	}

	ctx, cancel := tCtx.cancellable(0)
	err := registry.Register(
		testResourceType, v1.OperationPut,
		func(opts ctrl.Options) (ctrl.Controller, error) {
			return testCtrl, nil
		}, ctrl.Options{DatabaseClient: tCtx.mockSC})
	require.NoError(t, err)

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

	cancel()
	<-done

	require.Equal(t, 0, tCtx.internalQ.Len(), "message should be finished without overwriting the terminal status")
}

func TestStart_MaxConcurrency(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testOperationStatus, nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	registry := NewControllerRegistry()
	worker := New(Options{DequeueIntervalDuration: defaultTestDequeueInterval}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	// register test controller.
	cnt := &atomic.Int32{}
	maxConcurrency := &atomic.Int32{}
	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
		fn: func(ctx context.Context) (ctrl.Result, error) {
			cnt.Add(1)
			if maxConcurrency.Load() < cnt.Load() {
				maxConcurrency.Store(cnt.Load())
			}
			time.Sleep(100 * time.Millisecond)
			cnt.Add(-1)
			return ctrl.Result{}, nil
		},
	}
	ctx, cancel := tCtx.cancellable(time.Duration(0))
	err := registry.Register(
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
	for range testMessageCnt {
		testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
		testMessages = append(testMessages, testMessage)
		err = tCtx.testQueue.Enqueue(ctx, testMessage)
		require.NoError(t, err)
	}

	tCtx.drainQueueOrAssert(t)

	// Cancelling worker loop.
	cancel()
	<-done

	for i := range testMessageCnt {
		require.Equal(t, 1, testMessages[i].DequeueCount)
	}
	require.Equal(t, int32(defaultMaxOperationConcurrency), maxConcurrency.Load())
}

func TestStart_RunOperation(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(testOperationStatus, nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	registry := NewControllerRegistry()
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, registry)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
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
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)
	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewMockDeploymentProcessor(mctrl)
		},
	}

	testCtrl := &testAsyncController{
		BaseController: ctrl.NewBaseAsyncController(opts),
	}

	require.Equal(t, 1, tCtx.internalQ.Len())

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
	require.NoError(t, err)
	worker.runOperation(t.Context(), msg, testCtrl)

	// Ensure that message is finished.
	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
}

func TestRunOperation_ExtendMessageLock(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	// set up mocks
	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	tCtx.mockSM.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	worker := New(Options{}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
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

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
	old := msg.NextVisibleAt
	require.NoError(t, err)

	worker.runOperation(t.Context(), msg, testCtrl)

	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
	require.Greater(t, msg.NextVisibleAt.UnixNano(), old.UnixNano(), "message lock is extended")
}

func TestRunOperation_CancelContext(t *testing.T) {
	tCtx, _ := newTestContext(t, defaultTestLockTime)

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	worker := New(Options{}, nil, tCtx.testQueue, nil)

	// This test has a race condition with the worker loop trying to make an operation status
	// as failed. We can't use a mock because that might happen after the mock is destroyed (on test completion).
	//
	// Instead we use the in-memory store.

	opts := ctrl.Options{
		DatabaseClient: inmemorystore.NewClient(),
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

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
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
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
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
		DatabaseClient: tCtx.mockSC,
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

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
	require.NoError(t, err)
	worker.runOperation(t.Context(), msg, testCtrl)
	<-done

	require.Equal(t, 0, tCtx.internalQ.Len(), "message is finished")
}

// TestRunOperation_PanicController_Requeues verifies that when a controller panics on an attempt
// before the final retry, the message is left unfinished (so it is retried) and the operation is
// not completed.
func TestRunOperation_PanicController_Requeues(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	// MaxOperationRetryCount is high enough that the single delivery (DequeueCount == 1) is not the
	// final attempt, so the panic must not complete the operation - no status Update is expected.
	worker := New(Options{MaxOperationRetryCount: 3}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
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

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		worker.runOperation(tCtx.ctx, msg, testCtrl)
	})

	require.Equal(t, 1, tCtx.internalQ.Len(), "ensure that message is not finished so it is retried")
}

// TestRunOperation_PanicController_FinalAttempt verifies that when a controller panics on the final
// retry, the operation is completed as Failed with the real panic reason (instead of being retried
// to exhaustion and reported with a generic "exceeded max retry count" message), and the message is
// finished.
func TestRunOperation_PanicController_FinalAttempt(t *testing.T) {
	tCtx, mctrl := newTestContext(t, defaultTestLockTime)
	defer mctrl.Finish()

	testMessage := genTestMessage(uuid.New(), ctrl.DefaultAsyncOperationTimeout)
	err := tCtx.testQueue.Enqueue(tCtx.ctx, testMessage)
	require.NoError(t, err)

	tCtx.mockSC.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, id string, _ ...database.GetOptions) (*database.Object, error) {
			return newTestResourceObject(), nil
		}).AnyTimes()
	tCtx.mockSC.EXPECT().Save(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	var terminalErr *v1.ErrorDetails
	tCtx.mockSM.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(v1.ProvisioningStateFailed), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ resources.ID, _ uuid.UUID, _ v1.ProvisioningState, _ *time.Time, opErr *v1.ErrorDetails) error {
			terminalErr = opErr
			return nil
		}).Times(1)

	// MaxOperationRetryCount == 1 makes the single delivery (DequeueCount == 1) the final attempt.
	worker := New(Options{MaxOperationRetryCount: 1}, tCtx.mockSM, tCtx.testQueue, nil)

	opts := ctrl.Options{
		DatabaseClient: tCtx.mockSC,
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

	msg, err := tCtx.testQueue.Dequeue(tCtx.ctx, queue.QueueClientConfig{})
	require.NoError(t, err)

	require.NotPanics(t, func() {
		worker.runOperation(tCtx.ctx, msg, testCtrl)
	})

	require.Equal(t, 0, tCtx.internalQ.Len(), "ensure that the message is finished on the final attempt")
	require.NotNil(t, terminalErr, "the panic reason must be recorded as the terminal failure")
	require.Equal(t, v1.CodeInternal, terminalErr.Code)
	require.Contains(t, terminalErr.Message, "don't panic")
}

// ownedController is a test controller whose resources name the operation that owns them in "owner".
type ownedController struct {
	testAsyncController
}

func (c *ownedController) OwnsResource(resource *database.Object, request *ctrl.Request) (bool, error) {
	return resource.Data.(map[string]any)["owner"] == request.OperationID.String(), nil
}

// raceDB runs before once, on the first save of a resource, before it saves.
type raceDB struct {
	database.Client
	before func()
	once   sync.Once
}

func (r *raceDB) Save(ctx context.Context, obj *database.Object, options ...database.SaveOptions) error {
	r.once.Do(r.before)
	return r.Client.Save(ctx, obj, options...)
}

// settleFixture runs the worker on one operation, A, of a resource that may show a newer operation, B. It uses the
// real in-memory store and status manager.
type settleFixture struct {
	t          *testing.T
	db         database.Client
	sm         manager.StatusManager
	queue      *inmemory.Client
	internalQ  *inmemory.InmemQueue
	resourceID resources.ID
	opA, opB   uuid.UUID
}

func newSettleFixture(t *testing.T, lockTime time.Duration, owner string) *settleFixture {
	resourceID, err := resources.ParseResource("/planes/radius/local/resourceGroups/test-rg/providers/Applications.Core/environments/env0")
	require.NoError(t, err)
	internalQ := inmemory.NewInMemQueue(lockTime)
	f := &settleFixture{
		t:          t,
		db:         inmemorystore.NewClient(),
		queue:      inmemory.New(internalQ),
		internalQ:  internalQ,
		resourceID: resourceID,
		opA:        uuid.New(),
		opB:        uuid.New(),
	}
	// The status manager queues on its own queue, so the test controls the message the worker gets.
	f.sm = manager.New(f.db, inmemory.New(inmemory.NewInMemQueue(lockTime)), v1.LocationGlobal)
	sCtx := &v1.ARMRequestContext{ResourceID: resourceID, OperationID: f.opA, OperationType: v1.OperationType{Type: testResourceType, Method: v1.OperationPut}}
	require.NoError(t, f.sm.QueueAsyncOperation(t.Context(), sCtx, manager.QueueOperationOptions{}))

	if owner == "A" {
		owner = f.opA.String()
	} else {
		owner = f.opB.String()
	}
	f.saveResource(owner)
	return f
}

// saveResource saves the resource as the frontend would when it starts the operation owner.
func (f *settleFixture) saveResource(owner string) {
	f.t.Helper()
	require.NoError(f.t, f.db.Save(f.t.Context(), &database.Object{
		ID:   f.resourceID.String(),
		Data: map[string]any{"id": f.resourceID.String(), "provisioningState": string(v1.ProvisioningStateProvisioning), "owner": owner},
	}))
}

// run queues operation A with dequeueCount and timeout, and runs the worker until A's status is terminal and its
// message is finished.
func (f *settleFixture) run(asyncCtrl ctrl.Controller, dequeueCount int, timeout time.Duration) *manager.Status {
	f.t.Helper()
	registry := NewControllerRegistry()
	require.NoError(f.t, registry.Register(testResourceType, v1.OperationPut, func(ctrl.Options) (ctrl.Controller, error) { return asyncCtrl, nil }, ctrl.Options{DatabaseClient: f.db}))
	w := New(Options{MaxOperationRetryCount: 2, DequeueIntervalDuration: defaultTestDequeueInterval}, f.sm, f.queue, registry)

	msg := queue.NewMessage(&ctrl.Request{
		OperationID:      f.opA,
		OperationType:    "APPLICATIONS.CORE/ENVIRONMENTS|PUT",
		ResourceID:       f.resourceID.String(),
		OperationTimeout: &timeout,
	})
	require.NoError(f.t, f.queue.Enqueue(f.t.Context(), msg))
	msg.DequeueCount = dequeueCount

	ctx, cancel := context.WithCancel(f.t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NoError(f.t, w.Start(ctx))
	}()
	var status *manager.Status
	require.Eventually(f.t, func() bool {
		var err error
		status, err = f.sm.Get(f.t.Context(), f.resourceID, f.opA)
		return err == nil && status.Status.IsTerminal() && f.internalQ.Len() == 0
	}, 10*time.Second, time.Millisecond)
	cancel()
	<-done
	return status
}

func (f *settleFixture) resource() map[string]any {
	f.t.Helper()
	obj, err := f.db.Get(f.t.Context(), f.resourceID.String())
	require.NoError(f.t, err)
	return obj.Data.(map[string]any)
}

// TestStart_SettleReplacedOperation checks that when the worker settles an operation without the controller's result,
// on the retry limit or the operation timeout, it leaves a resource that a newer operation owns unchanged, and still
// settles the operation's own status.
func TestStart_SettleReplacedOperation(t *testing.T) {
	paths := map[string]struct {
		dequeueCount int
		timeout      time.Duration
		state        v1.ProvisioningState
		message      string
	}{
		"retry limit": {dequeueCount: 3, timeout: time.Minute, state: v1.ProvisioningStateFailed, message: "exceeded max retry count"},
		"timeout":     {timeout: 10 * time.Millisecond, state: v1.ProvisioningStateCanceled, message: "has timed out"},
	}
	owners := map[string]struct {
		owner   string
		checker bool
		// replaced means the resource keeps the newer operation's state.
		replaced bool
	}{
		"replaced":   {owner: "B", checker: true, replaced: true},
		"owned":      {owner: "A", checker: true},
		"no checker": {owner: "B"},
	}
	for pathName, path := range paths {
		for ownerName, owner := range owners {
			t.Run(pathName+"/"+ownerName, func(t *testing.T) {
				f := newSettleFixture(t, time.Minute, owner.owner)
				base := testAsyncController{
					BaseController: ctrl.NewBaseAsyncController(ctrl.Options{DatabaseClient: f.db}),
					fn: func(ctx context.Context) (ctrl.Result, error) {
						<-ctx.Done()
						return ctrl.Result{}, ctx.Err()
					},
				}
				var asyncCtrl ctrl.Controller = &base
				if owner.checker {
					asyncCtrl = &ownedController{testAsyncController: base}
				}
				before := f.resource()

				status := f.run(asyncCtrl, path.dequeueCount, path.timeout)
				require.Equal(t, path.state, status.Status)
				require.Contains(t, status.Error.Message, path.message)
				if owner.replaced {
					require.Equal(t, before, f.resource())
				} else {
					require.Equal(t, string(path.state), f.resource()["provisioningState"])
				}
			})
		}
	}
}

// TestStart_SettleRacesNewerOperation checks that a newer operation that takes the resource between the worker's
// ownership check and its save is not overwritten: the save fails on the ETag, and the redelivered message finds the
// newer owner.
func TestStart_SettleRacesNewerOperation(t *testing.T) {
	f := newSettleFixture(t, 50*time.Millisecond, "A")
	race := &raceDB{Client: f.db, before: func() { f.saveResource(f.opB.String()) }}
	asyncCtrl := &ownedController{testAsyncController: testAsyncController{BaseController: ctrl.NewBaseAsyncController(ctrl.Options{DatabaseClient: race})}}

	status := f.run(asyncCtrl, 3, time.Minute)
	require.Equal(t, v1.ProvisioningStateFailed, status.Status)
	require.Equal(t, string(v1.ProvisioningStateProvisioning), f.resource()["provisioningState"])
	require.Equal(t, f.opB.String(), f.resource()["owner"])
}

// TestUpdateResourceState_OwnershipError checks that the resource is not saved when the controller can't tell whether
// the operation owns it, and the error is returned so the message is retried.
func TestUpdateResourceState_OwnershipError(t *testing.T) {
	f := newSettleFixture(t, time.Minute, "A")
	before := f.resource()
	asyncCtrl := &failingOwner{testAsyncController{BaseController: ctrl.NewBaseAsyncController(ctrl.Options{DatabaseClient: f.db})}}
	err := updateResourceState(t.Context(), asyncCtrl, &ctrl.Request{OperationID: f.opA}, f.resourceID.String(), v1.ProvisioningStateFailed)
	require.ErrorContains(t, err, "cannot read the owner")
	require.Equal(t, before, f.resource())
}

type failingOwner struct {
	testAsyncController
}

func (c *failingOwner) OwnsResource(*database.Object, *ctrl.Request) (bool, error) {
	return false, errors.New("cannot read the owner")
}
