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
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/logging"
	"github.com/radius-project/radius/pkg/metrics"
	"github.com/radius-project/radius/pkg/trace"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

const (
	// defaultMaxOperationConcurrency is the default maximum concurrency to process async request operation.
	defaultMaxOperationConcurrency = 10

	// defaultMaxOperationRetryCount is the default maximum retry count to process async operation.
	defaultMaxOperationRetryCount = 3

	// messageExtendMargin is the default margin duration before extending message lock.
	defaultMessageExtendMargin = time.Duration(30) * time.Second

	// minMessageLockDuration is the default minimum duration of message lock duration.
	defaultMinMessageLockDuration = time.Duration(5) * time.Second

	// deduplicationDuration is the default duration for the deduplication detection.
	defaultDeduplicationDuration = time.Duration(30) * time.Second

	// defaultDequeueInterval is the default duration for the dequeue interval.
	defaultDequeueInterval = time.Duration(200) * time.Millisecond
)

// Options configures AsyncRequestProcessorWorker
type Options struct {
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency int

	// MaxOperationRetryCount is the maximum retry count to process async request operation.
	MaxOperationRetryCount int

	// MessageExtendMargin is the margin duration for clock skew before extending message lock.
	MessageExtendMargin time.Duration

	// MinMessageLockDuration is the minimum duration of message lock duration.
	MinMessageLockDuration time.Duration

	// DeduplicationDuration is the duration for the deduplication detection.
	DeduplicationDuration time.Duration

	// DequeueIntervalDuration is the duration for the dequeue interval.
	DequeueIntervalDuration time.Duration
}

// AsyncRequestProcessWorker is the worker to process async requests.
type AsyncRequestProcessWorker struct {
	options      Options
	sm           manager.StatusManager
	registry     *ControllerRegistry
	requestQueue queue.Client

	sem *semaphore.Weighted
}

// New creates AsyncRequestProcessWorker server instance.
func New(
	options Options,
	sm manager.StatusManager,
	qu queue.Client,
	ctrlRegistry *ControllerRegistry) *AsyncRequestProcessWorker {
	if options.MaxOperationConcurrency == 0 {
		options.MaxOperationConcurrency = defaultMaxOperationConcurrency
	}
	if options.MaxOperationRetryCount == 0 {
		options.MaxOperationRetryCount = defaultMaxOperationRetryCount
	}
	if options.MessageExtendMargin == time.Duration(0) {
		options.MessageExtendMargin = defaultMessageExtendMargin
	}
	if options.MinMessageLockDuration == time.Duration(0) {
		options.MinMessageLockDuration = defaultMinMessageLockDuration
	}
	if options.DeduplicationDuration == time.Duration(0) {
		options.DeduplicationDuration = defaultDeduplicationDuration
	}
	if options.DequeueIntervalDuration == time.Duration(0) {
		options.DequeueIntervalDuration = defaultDequeueInterval
	}

	return &AsyncRequestProcessWorker{
		options:      options,
		sm:           sm,
		registry:     ctrlRegistry,
		requestQueue: qu,
		sem:          semaphore.NewWeighted(int64(options.MaxOperationConcurrency)),
	}
}

// Start starts worker's message loop - it starts a loop to process messages from a queue concurrently, and handles deduplication, updating
// resource and operation status, and running the operation. It returns an error if it fails to start the dequeuer.
func (w *AsyncRequestProcessWorker) Start(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	msgCh, err := queue.StartDequeuer(ctx, w.requestQueue, queue.WithDequeueInterval(w.options.DequeueIntervalDuration))
	if err != nil {
		return err
	}

	// this loop will run until msgCh is closed (or when ctx is canceled)
	for msg := range msgCh {
		// This semaphore will maintain the number of go routines to process the messages concurrently.
		if err := w.sem.Acquire(ctx, 1); err != nil {
			break
		}

		go func(msgreq *queue.Message) {
			defer w.sem.Release(1)

			op := &ctrl.Request{}
			if err := json.Unmarshal(msgreq.Data, op); err != nil {
				logger.Error(err, "failed to unmarshal queue message.")
				return
			}

			reqCtx := trace.WithTraceparent(ctx, op.TraceparentID)

			// Populate the default attributes in the current context so all logs will have these fields.
			reqCtx = ucplog.WrapLogContext(reqCtx,
				logging.LogFieldResourceID, op.ResourceID,
				logging.LogFieldOperationID, op.OperationID,
				logging.LogFieldOperationType, op.OperationType,
				logging.LogFieldDequeueCount, msgreq.DequeueCount)

			opLogger := ucplog.FromContextOrDiscard(reqCtx)

			armReqCtx, err := op.ARMRequestContext()
			if err != nil {
				opLogger.Error(err, "failed to get ARM request context.")
				return
			}
			reqCtx = v1.WithARMRequestContext(reqCtx, armReqCtx)

			asyncCtrl, err := w.registry.Get(reqCtx, armReqCtx.OperationType)
			if err != nil {
				opLogger.Error(err, "failed to get async controller.")
				if err := w.requestQueue.FinishMessage(reqCtx, msgreq); err != nil {
					opLogger.Error(err, "failed to finish the message")
				}
				return
			}

			if asyncCtrl == nil {
				opLogger.Error(nil, "cannot process unknown operation: "+armReqCtx.OperationType.String())
				if err := w.requestQueue.FinishMessage(reqCtx, msgreq); err != nil {
					opLogger.Error(err, "failed to finish the message")
				}
				return
			}

			if msgreq.DequeueCount > w.options.MaxOperationRetryCount {
				errMsg := fmt.Sprintf("exceeded max retry count to process async operation message: %d", msgreq.DequeueCount)
				opLogger.Error(nil, errMsg)
				failed := ctrl.NewFailedResult(v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: errMsg,
				})
				w.completeOperation(reqCtx, msgreq, failed, asyncCtrl.StorageClient())
				return
			}

			// TODO: Handle the edge cases:
			// 1. The same message is delivered twice in multiple instances.
			// 2. provisioningState is not matched between resource and operationStatuses

			dup, err := w.isDuplicated(reqCtx, op.ResourceID, op.OperationID)
			if err != nil {
				opLogger.Error(err, "failed to check potential deduplication.")
				return
			}
			if dup {
				opLogger.Info("duplicated message detected")
				return
			}

			if err = w.updateResourceAndOperationStatus(reqCtx, asyncCtrl.StorageClient(), op, v1.ProvisioningStateUpdating, nil); err != nil {
				return
			}

			w.runOperation(reqCtx, msgreq, asyncCtrl)
		}(msg)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, asyncCtrl ctrl.Controller) {
	ctx, span := trace.StartConsumerSpan(ctx, "worker.runOperation receive", trace.BackendTracerName)
	defer span.End()
	logger := ucplog.FromContextOrDiscard(ctx)

	asyncReq := &ctrl.Request{}
	if err := json.Unmarshal(message.Data, asyncReq); err != nil {
		logger.Error(err, "failed to unmarshal queue message.")
		return
	}
	asyncReqCtx, opCancel := context.WithCancel(ctx)
	// Ensure that asyncReqCtx context is cancelled when runOperation returns.
	// That is, cancelling asyncReqCtx signals to ctrl.Run() to cancel the execution,
	// resulting in completing the go-routine calling ctrl.Run() when runOperation returns.
	defer opCancel()

	opDone := make(chan struct{}, 1)
	opStartAt := time.Now()

	// Start new go routine to cancel and timeout async operation.
	go func() {
		defer func(done chan struct{}) {
			close(done)
			if err := recover(); err != nil {
				msg := fmt.Errorf("recovering from panic %v: %s", err, debug.Stack())
				logger.Error(msg, "recovering from panic")

				// When backend controller has a critical bug such as nil reference, asyncCtrl.Run() is panicking.
				// If this happens, the message is requeued after message lock time (5 mins).
				// After message lock is expired, message will be reprocessed 'w.options.MaxOperationRetryCount' times and
				// then complete the message and change provisioningState to 'Failed'. Meanwhile, PUT request will
				// be blocked.
			}
		}(opDone)

		logger.Info("Start processing operation.")
		result, err := asyncCtrl.Run(asyncReqCtx, asyncReq)
		// Update the result if an error is returned from the controller.
		// Check that the result is empty to ensure we don't override it, it shouldn't happen.
		// Controller should always either return non-empty error or non-empty result, but not both.
		if err != nil && result.Error == nil {
			armErr := extractError(err)
			result.SetFailed(armErr, false)
		}

		logger.Info("Operation returned", "success", result.Error == nil, "provisioningState", result.ProvisioningState(), "err", result.Error)

		// There are two cases when asyncReqCtx is canceled.
		// 1. When the operation is timed out, w.completeOperation will be called in L186
		// 2. When parent context is canceled or done, we need to requeue the operation to reprocess the request.
		// Such cases should not call w.completeOperation.
		if !errors.Is(asyncReqCtx.Err(), context.Canceled) {
			w.completeOperation(ctx, message, result, asyncCtrl.StorageClient())
		}
		trace.SetAsyncResultStatus(result, span)
	}()

	operationTimeoutAfter := time.After(asyncReq.Timeout())
	messageExtendAfter := w.getMessageExtendDuration(message.NextVisibleAt)

	for {
		select {
		case <-time.After(messageExtendAfter):
			if err := w.requestQueue.ExtendMessage(ctx, message); err != nil {
				logger.Error(err, "fails to extend message lock")
			} else {
				logger.Info("Extended message lock duration.", "nextVisibleTime", message.NextVisibleAt.UTC().String())
				metrics.DefaultAsyncOperationMetrics.RecordExtendedAsyncOperation(ctx, asyncReq)
			}
			messageExtendAfter = w.getMessageExtendDuration(message.NextVisibleAt)

		case <-operationTimeoutAfter:
			logger.Info("Cancelling async operation.")

			opCancel()
			errMessage := fmt.Sprintf("Operation (%s) has timed out because it was processing longer than %d s.", asyncReq.OperationType, int(asyncReq.Timeout().Seconds()))
			result := ctrl.NewCanceledResult(errMessage)
			result.Error.Target = asyncReq.ResourceID
			w.completeOperation(ctx, message, result, asyncCtrl.StorageClient())
			return

		case <-ctx.Done():
			logger.Info("Stopping processing async operation. This operation will be reprocessed.")
			return

		case <-opDone:
			// FIXME: Would this give me all the operations? No matter if it is successful or cancelled or failed?
			metrics.DefaultAsyncOperationMetrics.RecordAsyncOperationDuration(ctx, asyncReq, opStartAt)

			opEndAt := time.Now()
			logger.Info("End processing operation.", "startAt", opStartAt.UTC(), "endAt", opEndAt.UTC(), "duration", opEndAt.Sub(opStartAt))
			return
		}
	}
}

func extractError(err error) v1.ErrorDetails {
	if clientErr, ok := err.(*v1.ErrClientRP); ok {
		return v1.ErrorDetails{Code: clientErr.Code, Message: clientErr.Message}
	} else {
		return v1.ErrorDetails{Code: v1.CodeInternal, Message: err.Error()}
	}
}

func (w *AsyncRequestProcessWorker) completeOperation(ctx context.Context, message *queue.Message, result ctrl.Result, sc store.StorageClient) {
	logger := ucplog.FromContextOrDiscard(ctx)
	req := &ctrl.Request{}
	if err := json.Unmarshal(message.Data, req); err != nil {
		logger.Error(err, "failed to unmarshal queue message.")
		return
	}

	err := w.updateResourceAndOperationStatus(ctx, sc, req, result.ProvisioningState(), result.Error)
	if err != nil {
		logger.Error(err, "failed to update resource and/or operation status")
		return
	}

	// Finish the message only if Requeue is false. Otherwise, AsyncRequestProcessWorker will requeue the message and process it again.
	if !result.Requeue {
		if err := w.requestQueue.FinishMessage(ctx, message); err != nil {
			logger.Error(err, "failed to finish the message")
		}
	}

	metrics.DefaultAsyncOperationMetrics.RecordAsyncOperation(ctx, req, &result)
}

func (w *AsyncRequestProcessWorker) updateResourceAndOperationStatus(ctx context.Context, sc store.StorageClient, req *ctrl.Request, state v1.ProvisioningState, opErr *v1.ErrorDetails) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	rID, err := resources.ParseResource(req.ResourceID)
	if err != nil {
		logger.Error(err, "failed to parse resource ID")
		return err
	}

	err = updateResourceState(ctx, sc, rID.String(), state)
	if errors.Is(err, &store.ErrNotFound{}) {
		logger.Info("failed to update the provisioningState in resource because it no longer exists.")
	} else if err != nil {
		logger.Error(err, "failed to update the provisioningState in resource.")
		return err
	}

	// Otherwise we update the operationStatus to the result.
	now := time.Now().UTC()
	err = w.sm.Update(ctx, rID, req.OperationID, state, &now, opErr)
	if err != nil {
		logger.Error(err, "failed to update operationstatus", "operationID", req.OperationID.String())
		return err
	}

	return nil
}

func (w *AsyncRequestProcessWorker) isDuplicated(ctx context.Context, resourceID string, operationID uuid.UUID) (bool, error) {
	rID, err := resources.ParseResource(resourceID)
	if err != nil {
		return false, err
	}

	status, err := w.sm.Get(ctx, rID, operationID)
	if err != nil {
		return false, err
	}

	// 1. If the operation is in updating state and the last updated time is within the deduplication duration, we consider it as a duplicated operation.
	// 2. If the operation is in terminal state, we consider it as a duplicated operation.
	if (status.Status == v1.ProvisioningStateUpdating && status.LastUpdatedTime.IsZero() &&
		status.LastUpdatedTime.Add(w.options.DeduplicationDuration).After(time.Now().UTC())) ||
		status.Status.IsTerminal() {
		return true, nil
	}

	return false, nil
}

func (w *AsyncRequestProcessWorker) getMessageExtendDuration(visibleAt time.Time) time.Duration {
	d := time.Until(visibleAt.Add(-w.options.MessageExtendMargin))
	if d <= 0 {
		return w.options.MinMessageLockDuration
	}
	return d
}

func updateResourceState(ctx context.Context, sc store.StorageClient, id string, state v1.ProvisioningState) error {
	obj, err := sc.Get(ctx, id)
	if err != nil {
		return err
	}

	objmap := obj.Data.(map[string]any)
	pState, ok := objmap["provisioningState"].(string)
	if ok && strings.EqualFold(pState, string(state)) {
		// Do not update it if provisioning state is already the target state.
		// This happens when redeploying worker can stop completing message.
		// So, provisioningState in Resource is updated but not in operationStatus record.
		return nil
	}

	objmap["provisioningState"] = string(state)

	err = sc.Save(ctx, obj, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	return nil
}
