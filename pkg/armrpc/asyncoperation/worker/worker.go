// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/logging"
	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/trace"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"
)

const (
	// defaultMaxOperationConcurrency is the default maximum concurrency to process async request operation.
	defaultMaxOperationConcurrency = 3

	// defaultMaxOperationRetryCount is the default maximum retry count to process async operation.
	defaultMaxOperationRetryCount = 3

	// messageExtendMargin is the default margin duration before extending message lock.
	defaultMessageExtendMargin = time.Duration(30) * time.Second

	// minMessageLockDuration is the default minimum duration of message lock duration.
	defaultMinMessageLockDuration = time.Duration(5) * time.Second

	// deduplicationDuration is the default duration for the deduplication detection.
	defaultDeduplicationDuration = time.Duration(30) * time.Second
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

	return &AsyncRequestProcessWorker{
		options:      options,
		sm:           sm,
		registry:     ctrlRegistry,
		requestQueue: qu,
		sem:          semaphore.NewWeighted(int64(options.MaxOperationConcurrency)),
	}
}

// Start starts worker's message loop.
func (w *AsyncRequestProcessWorker) Start(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	msgCh, err := queue.StartDequeuer(ctx, w.requestQueue)
	if err != nil {
		return err
	}

	// this loop will run until msgCh is closed (or when ctx is canceled)
	for msg := range msgCh {
		// This semaphore will maintain the number of go routines to process the messages concurrently.
		if err := w.sem.Acquire(ctx, 1); err != nil {
			break
		}

		fmt.Printf("WORKER - %s - Message Got Picked Up\n", msg.ID)

		go func(msgreq *queue.Message) {
			defer w.sem.Release(1)

			op := &ctrl.Request{}
			if err := json.Unmarshal(msgreq.Data, op); err != nil {
				logger.Error(err, "failed to unmarshal queue message.")
				return
			}

			reqCtx := trace.WithTraceparent(ctx, op.TraceparentID)
			// Populate the default attributes in the current context so all logs will have these fields.
			opLogger := ucplog.FromContextOrDiscard(reqCtx).WithValues(
				logging.LogFieldResourceID, op.ResourceID,
				logging.LogFieldOperationID, op.OperationID,
				logging.LogFieldOperationType, op.OperationType,
				logging.LogFieldDequeueCount, msgreq.DequeueCount,
			)

			armReqCtx, err := op.ARMRequestContext()
			if err != nil {
				opLogger.Error(err, "failed to get ARM request context.")
				return
			}
			reqCtx = v1.WithARMRequestContext(reqCtx, armReqCtx)

			opType, ok := v1.ParseOperationType(armReqCtx.OperationType)
			if !ok {
				opLogger.V(ucplog.Error).Info("WORKER - failed to parse operation type.")
				return
			}

			asyncCtrl := w.registry.Get(opType)
			if asyncCtrl == nil {
				opLogger.V(ucplog.Error).Info("WORKER - cannot process the unknown operation: " + opType.String())
				if err := w.requestQueue.FinishMessage(reqCtx, msgreq); err != nil {
					opLogger.Error(err, "WORKER - failed to finish the message")
				}
				return
			}

			if msgreq.DequeueCount > w.options.MaxOperationRetryCount {
				errMsg := fmt.Sprintf("exceeded max retry count to process async operation message: %d", msgreq.DequeueCount)
				opLogger.V(ucplog.Error).Info(errMsg)
				fmt.Printf("WORKER - %s - Dequeue Limit Reached\n", op.OperationID)
				failed := ctrl.NewFailedResult(v1.ErrorDetails{
					Code:    v1.CodeInternal,
					Message: errMsg,
				})
				w.completeOperation(ctx, msgreq, failed, asyncCtrl.StorageClient())
				return
			}

			// TODO: Handle the edge cases:
			// 1. The same message is delivered twice in multiple instances.
			// 2. provisioningState is not matched between resource and operationStatuses

			dup, err := w.isDuplicated(ctx, asyncCtrl.StorageClient(), op.ResourceID, op.OperationID)
			if err != nil {
				opLogger.Error(err, "WORKER - failed to check potential deduplication.")
				return
			}
			if dup {
				opLogger.V(ucplog.Warn).Info("WORKER - duplicated message detected")
				return
			}

			if err = w.updateResourceAndOperationStatus(reqCtx, asyncCtrl.StorageClient(), op, v1.ProvisioningStateUpdating, nil); err != nil {
				return
			}

			w.runOperation(reqCtx, msgreq, asyncCtrl)
		}(msg)
	}

	logger.Info("WORKER - Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, asyncCtrl ctrl.Controller) {
	ctx, span := trace.StartConsumerSpan(ctx, "worker.runOperation receive", trace.BackendTracerName)

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
				msg := fmt.Sprintf("WORKER - recovering from panic %v: %s", err, debug.Stack())
				logger.V(ucplog.Error).Info(msg)
				fmt.Printf("WORKER - %s - Panic - Error - %v\n", asyncReq.OperationID, err)
				// When backend controller has a critical bug such as nil reference, asyncCtrl.Run() is panicking.
				// If this happens, the message is requeued after message lock time (5 mins).
				// After message lock is expired, message will be reprocessed 'w.options.MaxOperationRetryCount' times and
				// then complete the message and change provisioningState to 'Failed'. Meanwhile, PUT request will
				// be blocked.
			}
		}(opDone)

		logger.Info("WORKER - %s - START", "operationID", asyncReq.OperationID)
		fmt.Printf("WORKER - %s - START\n", asyncReq.OperationID)

		logger.Info("WORKER - %s - START - ResourceID - %s", "operationID", asyncReq.OperationID, "resourceID", asyncReq.ResourceID)
		fmt.Printf("WORKER - %s - START - ResourceID - %s\n", asyncReq.OperationID, asyncReq.ResourceID)

		result, err := asyncCtrl.Run(asyncReqCtx, asyncReq)
		// There are two cases when asyncReqCtx is canceled.
		// 1. When the operation is timed out, w.completeOperation will be called in L186
		// 2. When parent context is canceled or done, we need to requeue the operation to reprocess the request.
		// Such cases should not call w.completeOperation.
		if !errors.Is(asyncReqCtx.Err(), context.Canceled) {
			if err != nil {
				armErr := extractError(err)
				result.SetFailed(armErr, false)
				logger.Error(err, "WORKER - Operation Failed")
				fmt.Printf("WORKER - %s - Operation Failed\n", asyncReq.OperationID)
				fmt.Printf("WORKER - %s - Operation Failed - Err - %s\n", asyncReq.OperationID, err.Error())
			}
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
				logger.Error(err, "WORKER - fails to extend message lock")
				fmt.Printf("WORKER - %s - fails to extend message lock\n", asyncReq.OperationID)
			} else {
				logger.Info("WORKER - Extended message lock duration.", "nextVisibleTime", message.NextVisibleAt.UTC().String())
				fmt.Printf("WORKER - %s - Extended message lock duration. NextVisibleTime - %s\n",
					asyncReq.OperationID, message.NextVisibleAt.UTC().String())
				metrics.DefaultAsyncOperationMetrics.RecordExtendedAsyncOperation(ctx, asyncReq)
			}
			messageExtendAfter = w.getMessageExtendDuration(message.NextVisibleAt)

		case <-operationTimeoutAfter:
			logger.Info("WORKER - Cancelling async operation.")

			opCancel()
			errMessage := fmt.Sprintf("Operation (%s) has timed out because it was processing longer than %d s.", asyncReq.OperationType, int(asyncReq.Timeout().Seconds()))
			result := ctrl.NewCanceledResult(errMessage)
			result.Error.Target = asyncReq.ResourceID
			w.completeOperation(ctx, message, result, asyncCtrl.StorageClient())
			span.End()
			return

		case <-ctx.Done():
			logger.Info("WORKER - Stopping processing async operation. This operation will be reprocessed.")
			fmt.Printf("WORKER - %s - Stopping processing async operation. This operation will be reprocessed.", asyncReq.OperationID)
			span.End()
			return

		case <-opDone:
			// FIXME: Would this give me all the operations? No matter if it is successful or cancelled or failed?
			metrics.DefaultAsyncOperationMetrics.RecordAsyncOperationDuration(ctx, asyncReq, opStartAt)

			opEndAt := time.Now()
			logger.Info("WORKER - End processing operation.", "startAt", opStartAt.UTC(), "endAt", opEndAt.UTC(), "duration", opEndAt.Sub(opStartAt))
			fmt.Printf("WORKER - %s - END\n", asyncReq.OperationID)
			span.End()
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
		logger.Error(err, "WORKER - failed to unmarshal queue message.")
		fmt.Printf("WORKER - %s - failed to unmarshal queue message\n", req.OperationID)
		return
	}

	err := w.updateResourceAndOperationStatus(ctx, sc, req, result.ProvisioningState(), result.Error)
	if err != nil {
		logger.Error(err, "WORKER - failed to update resource and/or operation status")
		fmt.Printf("WORKER - %s - failed to update resource and/or operation status\n", req.OperationID)
		return
	}

	// Finish the message only if Requeue is false. Otherwise, AsyncRequestProcessWorker will requeue the message and process it again.
	if !result.Requeue {
		if err := w.requestQueue.FinishMessage(ctx, message); err != nil {
			logger.Error(err, "WORKER - failed to finish the message")
			fmt.Printf("WORKER - %s - Failed to finish the message\n", req.OperationID)
		}
	}

	fmt.Printf("WORKER - %s - Operation Completed - Status - %s\n", req.OperationID, result.ProvisioningState())

	metrics.DefaultAsyncOperationMetrics.RecordAsyncOperation(ctx, req, &result)
}

func (w *AsyncRequestProcessWorker) updateResourceAndOperationStatus(ctx context.Context, sc store.StorageClient, req *ctrl.Request, state v1.ProvisioningState, opErr *v1.ErrorDetails) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	rID, err := resources.ParseResource(req.ResourceID)
	if err != nil {
		logger.Error(err, "WORKER - failed to parse resource ID")
		fmt.Printf("WORKER - %s - Failed to parse resource ID\n", req.OperationID)
		return err
	}

	opType, _ := v1.ParseOperationType(req.OperationType)

	err = updateResourceState(ctx, sc, rID.String(), state)
	if err != nil && !(opType.Method == http.MethodDelete && errors.Is(&store.ErrNotFound{}, err)) {
		logger.Error(err, "WORKER - failed to update the provisioningState in resource.")
		fmt.Printf("WORKER - %s - Failed to update the provisioningState in resource\n", req.OperationID)
		return err
	}

	// Otherwise we update the operationStatus to the result.
	now := time.Now().UTC()
	err = w.sm.Update(ctx, rID, req.OperationID, state, &now, opErr)
	if err != nil {
		logger.Error(err, "WORKER - failed to update operationstatus", "operationID", req.OperationID.String())
		fmt.Printf("WORKER - %s - Failed to update operationStatus\n", req.OperationID)
		return err
	}

	fmt.Printf("WORKER - %s - Update resource and operation status\n", req.OperationID)

	return nil
}

func (w *AsyncRequestProcessWorker) isDuplicated(ctx context.Context, sc store.StorageClient, resourceID string, operationID uuid.UUID) (bool, error) {
	rID, err := resources.ParseResource(resourceID)
	if err != nil {
		return false, err
	}

	status, err := w.sm.Get(ctx, rID, operationID)
	if err != nil {
		return false, err
	}

	if (status.Status == v1.ProvisioningStateUpdating && status.LastUpdatedTime.IsZero() &&
		status.LastUpdatedTime.Add(w.options.DeduplicationDuration).After(time.Now().UTC())) ||
		// This means that this message has already been processed
		// TODO: Should we try if the status is Failed or Cancelled?
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

	fmt.Printf("WORKER - Update resource state - id: %s\n", id)
	err = sc.Save(ctx, obj, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	return nil
}
