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

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	queue "github.com/project-radius/radius/pkg/ucp/queue/client"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"golang.org/x/sync/semaphore"
)

var (
	errPropertiesNotFound        = errors.New("properties object not found")
	errProvisioningStateNotFound = errors.New("provisioningState property not found")
)

const (
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	// TOOD: make this concurrency configurable.
	MaxOperationConcurrency = 3

	// MaxDequeueCount is the maximum dequeue count which will be retried.
	MaxDequeueCount = 5

	// messageExtendMargin is the margin duration before extending message lock.
	messageExtendMargin = time.Duration(30) * time.Second

	// minMessageLockDuration is the minimum duration of message lock duration.
	minMessageLockDuration = time.Duration(5) * time.Second

	// deduplicationDuration is the duration for the deduplication detection.
	deduplicationDuration = time.Duration(30) * time.Second
)

type Options struct {
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
	return &AsyncRequestProcessWorker{
		options:      options,
		sm:           sm,
		registry:     ctrlRegistry,
		requestQueue: qu,

		sem: semaphore.NewWeighted(MaxOperationConcurrency),
	}
}

// Start starts worker's message loop.
func (w *AsyncRequestProcessWorker) Start(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

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

		go func(msgreq *queue.Message) {
			defer w.sem.Release(1)

			op := &ctrl.Request{}
			if err := json.Unmarshal(msgreq.Data, op); err != nil {
				logger.Error(err, "failed to unmarshal queue message.")
				return
			}

			opLogger := logger.WithValues(
				"OperationID", op.OperationID.String(),
				"OperationType", op.OperationType,
				"ResourceID", op.ResourceID,
				"CorrleationID", op.CorrelationID,
				"W3CTraceID", op.TraceparentID,
			)

			opType, ok := v1.ParseOperationType(op.OperationType)
			if !ok {
				opLogger.V(radlogger.Error).Info("failed to parse operation type.")
				return
			}

			ctrl := w.registry.Get(opType)

			if ctrl == nil {
				opLogger.V(radlogger.Error).Info("Unknown operation")
				if err := w.requestQueue.FinishMessage(ctx, msgreq); err != nil {
					logger.Error(err, "failed to finish the message which includes unknown operation.")
				}
				return
			}
			if msgreq.DequeueCount >= MaxDequeueCount {
				opLogger.V(radlogger.Error).Info(fmt.Sprintf("Exceed max retrycount: %d", msgreq.DequeueCount))
				if err := w.requestQueue.FinishMessage(ctx, msgreq); err != nil {
					logger.Error(err, "failed to finish the message which exceeds the max retry count.")
				}
				return
			}

			// TODO: Handle the edge cases:
			// 1. The same message is delivered twice in multiple instances.
			// 2. provisioningState is not matched between resource and operationStatuses

			dup, err := w.isDuplicated(ctx, ctrl.StorageClient(), op.ResourceID, op.OperationID)
			if err != nil {
				logger.Error(err, "failed to check potential deduplication.")
				return
			}
			if dup {
				opLogger.V(radlogger.Warn).Info("duplicated message detected")
				return
			}

			if err = w.updateResourceAndOperationStatus(ctx, ctrl.StorageClient(), op, v1.ProvisioningStateUpdating, nil); err != nil {
				return
			}

			opCtx := logr.NewContext(ctx, opLogger)
			w.runOperation(opCtx, msgreq, ctrl)
		}(msg)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, asyncCtrl ctrl.Controller) {
	logger := logr.FromContextOrDiscard(ctx)

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
				msg := fmt.Sprintf("recovering from panic %v: %s", err, debug.Stack())
				logger.V(radlogger.Fatal).Info(msg)
			}
		}(opDone)

		logger.Info("Start processing operation.")
		result, err := asyncCtrl.Run(asyncReqCtx, asyncReq)
		// There are two cases when asyncReqCtx is canceled.
		// 1. When the operation is timed out, w.completeOperation will be called in L186
		// 2. When parent context is canceled or done, we need to requeue the operation to reprocess the request.
		// Such cases should not call w.completeOperation.
		if !errors.Is(asyncReqCtx.Err(), context.Canceled) {
			if err != nil {
				result.SetFailed(armerrors.ErrorDetails{Code: armerrors.Internal, Message: err.Error()}, false)
			}
			w.completeOperation(ctx, message, result, asyncCtrl.StorageClient())
		}
	}()

	operationTimeoutAfter := time.After(asyncReq.Timeout())
	messageExtendAfter := getMessageExtendDuration(message.NextVisibleAt)

	for {
		select {
		case <-time.After(messageExtendAfter):
			if err := w.requestQueue.ExtendMessage(ctx, message); err != nil {
				logger.Error(err, "fails to extend message lock")
			} else {
				logger.Info("Extended message lock duration.", "NextVisibleTime", message.NextVisibleAt.UTC().String())
			}
			messageExtendAfter = getMessageExtendDuration(message.NextVisibleAt)

		case <-operationTimeoutAfter:
			logger.Info("Cancelling async operation.")

			opCancel()
			w.completeOperation(ctx, message, ctrl.NewCanceledResult("async operation timeout"), asyncCtrl.StorageClient())
			return

		case <-ctx.Done():
			logger.Info("Stopping processing async operation. This operation will be reprocessed.")
			return

		case <-opDone:
			opEndAt := time.Now()
			logger.Info("End processing operation.", "StartAt", opStartAt.UTC(), "EndAt", opEndAt.UTC(), "Duration", opEndAt.Sub(opStartAt))
			return
		}
	}
}

func (w *AsyncRequestProcessWorker) completeOperation(ctx context.Context, message *queue.Message, result ctrl.Result, sc store.StorageClient) {
	logger := logr.FromContextOrDiscard(ctx)
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
}

func (w *AsyncRequestProcessWorker) updateResourceAndOperationStatus(ctx context.Context, sc store.StorageClient, req *ctrl.Request, state v1.ProvisioningState, opErr *armerrors.ErrorDetails) error {
	logger := logr.FromContextOrDiscard(ctx)

	rID, err := resources.Parse(req.ResourceID)
	if err != nil {
		logger.Error(err, "failed to parse resource ID")
		return err
	}

	opType, _ := v1.ParseOperationType(req.OperationType)

	err = updateResourceState(ctx, sc, rID.String(), state)
	if err != nil && !(opType.Method == http.MethodDelete && errors.Is(&store.ErrNotFound{}, err)) {
		logger.Error(err, "failed to update the provisioningState in resource.")
		return err
	}

	// Otherwise we update the operationStatus to the result.
	now := time.Now().UTC()
	err = w.sm.Update(ctx, rID, req.OperationID, state, &now, opErr)
	if err != nil {
		logger.Error(err, "failed to update operationstatus", "OperationID", req.OperationID.String())
		return err
	}

	return nil
}

func (w *AsyncRequestProcessWorker) isDuplicated(ctx context.Context, sc store.StorageClient, resourceID string, operationID uuid.UUID) (bool, error) {
	rID, err := resources.Parse(resourceID)
	if err != nil {
		return false, err
	}

	status, err := w.sm.Get(ctx, rID, operationID)
	if err != nil {
		return false, err
	}

	if status.Status == v1.ProvisioningStateUpdating && status.LastUpdatedTime.IsZero() &&
		status.LastUpdatedTime.Add(deduplicationDuration).After(time.Now().UTC()) {
		return true, nil
	}

	return false, nil
}

func getMessageExtendDuration(visibleAt time.Time) time.Duration {
	d := time.Until(visibleAt.Add(-messageExtendMargin))
	if d <= 0 {
		return minMessageLockDuration
	}
	return d
}

func updateResourceState(ctx context.Context, sc store.StorageClient, id string, state v1.ProvisioningState) error {
	obj, err := sc.Get(ctx, id)
	if err != nil {
		return err
	}

	objmap := obj.Data.(map[string]interface{})
	objmap, ok := objmap["properties"].(map[string]interface{})
	if !ok {
		return errPropertiesNotFound
	}

	if pState, ok := objmap["provisioningState"].(string); !ok {
		return errProvisioningStateNotFound
	} else if strings.EqualFold(pState, string(state)) {
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
