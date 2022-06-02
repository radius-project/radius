// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
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
	MaxOperationConcurrency = 3

	// MaxDequeueCount is the maximum dequeue count which will be retried.
	MaxDequeueCount = 5

	// messageExtendMargin is the margin duration before extending message lock.
	messageExtendMargin = time.Duration(30) * time.Second

	// minMessageLockDuration is the minimum duration of message lock duration.
	minMessageLockDuration = time.Duration(5) * time.Second
)

// AsyncRequestProcessWorker is the worker to process async requests.
type AsyncRequestProcessWorker struct {
	options      hostoptions.HostOptions
	operationMgr asyncoperation.StatusManager
	registry     *ControllerRegistry
	requestQueue queue.Dequeuer

	sem *semaphore.Weighted
}

// NewAsyncRequestProcessWorker creates AsyncRequestProcessWorker server instance.
func NewAsyncRequestProcessWorker(
	options hostoptions.HostOptions,
	om asyncoperation.StatusManager,
	qu queue.Dequeuer,
	ctrlRegistry *ControllerRegistry) *AsyncRequestProcessWorker {
	return &AsyncRequestProcessWorker{
		options:      options,
		operationMgr: om,
		registry:     ctrlRegistry,
		requestQueue: qu,

		sem: semaphore.NewWeighted(MaxOperationConcurrency),
	}
}

// Start starts worker's message loop.
func (w *AsyncRequestProcessWorker) Start(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	ctx = hostoptions.WithContext(ctx, w.options.Config)
	msgCh, err := w.requestQueue.Dequeue(ctx)
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

			op := msgreq.Data.(*asyncoperation.Request)
			opLogger := logger.WithValues([]interface{}{
				"OperationID", op.OperationID.String(),
				"OperationType", op.OperationType,
				"ResourceID", op.ResourceID,
				"CorrleationID", op.CorrelationID,
				"W3CTraceID", op.TraceparentID,
			})

			opType, ok := asyncoperation.ParseOperationType(op.OperationType)
			if !ok {
				opLogger.V(radlogger.Error).Info("failed to parse operation type.")
				return
			}

			ctrl := w.registry.Get(opType)

			if ctrl == nil {
				opLogger.V(radlogger.Error).Info("Unknown operation")
				if err := msgreq.Finish(nil); err != nil {
					logger.Error(err, "failed to finish the message which includes unknown operation.")
				}
				return
			}
			if msgreq.DequeueCount >= MaxDequeueCount {
				opLogger.V(radlogger.Error).Info(fmt.Sprintf("Exceed max retrycount: %d", msgreq.DequeueCount))
				if err := msgreq.Finish(nil); err != nil {
					logger.Error(err, "failed to finish the message which exceeds the max retry count.")
				}
				return
			}

			// TODO: Handle the edge cases:
			// 1. The same message is delivered twice in multiple instances.
			// 2. provisioningState is not matched between resource and operationStatuses

			opCtx := logr.NewContext(ctx, opLogger)
			w.runOperation(opCtx, msgreq, ctrl)
		}(msg)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, ctrl asyncoperation.Controller) {
	logger := logr.FromContextOrDiscard(ctx)

	asyncReq := message.Data.(*asyncoperation.Request)
	asyncReqCtx, opCancel := context.WithCancel(ctx)
	defer opCancel()

	opDone := make(chan struct{}, 1)
	opStartAt := time.Now()

	// Start new go routine to cancel and timeout async operation.
	go func() {
		defer func(done chan struct{}) {
			opEndAt := time.Now()
			logger.Info("End processing operation.", "StartAt", opStartAt.UTC(), "EndAt", opEndAt.UTC(), "Duration", opEndAt.Sub(opStartAt))
			close(done)
			if err := recover(); err != nil {
				msg := fmt.Sprintf("recovering from panic %v: %s", err, debug.Stack())
				logger.V(radlogger.Fatal).Info(msg)
			}
		}(opDone)

		logger.Info("Start processing operation.")
		result, err := ctrl.Run(asyncReqCtx, asyncReq)
		// Do not update status if context is canceled already.
		if !errors.Is(asyncReqCtx.Err(), context.Canceled) {
			if err != nil {
				result.SetFailed(armerrors.ErrorDetails{Code: armerrors.Internal, Message: err.Error()}, false)
			}
			w.completeOperation(ctx, message, result, ctrl.StorageClient())
		}
	}()

	operationTimeoutAfter := time.After(asyncReq.Timeout())
	messageExtendAfter := getMessageExtendDuration(message.NextVisibleAt)

	for {
		select {
		case <-time.After(messageExtendAfter):
			if err := message.Extend(); err != nil {
				logger.Error(err, "fails to extend message lock")
			} else {
				logger.Info("Extended message lock duration.", "NextVisibleTime", message.NextVisibleAt.UTC().String())
			}
			messageExtendAfter = getMessageExtendDuration(message.NextVisibleAt)

		case <-operationTimeoutAfter:
			logger.Info("Cancelling async operation.")

			opCancel()
			w.completeOperation(ctx, message, asyncoperation.NewCanceledResult("async operation timeout"), ctrl.StorageClient())
			return

		case <-ctx.Done():
			logger.Info("Stopping processing async operation. This operation will be reprocessed.")
			return

		case <-opDone:
			logger.V(radlogger.Debug).Info("exiting the goroutine for async operation execution.")
			return
		}
	}
}

func (w *AsyncRequestProcessWorker) completeOperation(ctx context.Context, message *queue.Message, result asyncoperation.Result, sc store.StorageClient) {
	logger := logr.FromContextOrDiscard(ctx)
	req := message.Data.(*asyncoperation.Request)

	rID, err := resources.Parse(req.ResourceID)
	if err != nil {
		logger.Error(err, "failed to parse resource ID")
		return
	}

	if err = updateResourceState(ctx, sc, rID.String(), result.ProvisioningState()); err != nil {
		logger.Error(err, "failed to update the provisioningState in resource.")
		return
	}

	now := time.Now().UTC()
	err = w.operationMgr.Update(ctx, rID.RootScope(), req.OperationID, result.ProvisioningState(), &now, result.Error)
	if err != nil {
		logger.Error(err, "failed to update operationstatus", "OperationID", req.OperationID.String())
	}

	// Finish the message only if Requeue is false. Otherwise, AsyncRequestProcessWorker will requeue the message and process it again.
	if !result.Requeue {
		if err := message.Finish(nil); err != nil {
			logger.V(radlogger.Error).Info("failed to finish the message")
		}
	}
}

func getMessageExtendDuration(visibleAt time.Time) time.Duration {
	d := time.Until(visibleAt.Add(-messageExtendMargin))
	if d <= 0 {
		return minMessageLockDuration
	}
	return d
}

func updateResourceState(ctx context.Context, sc store.StorageClient, id string, state basedatamodel.ProvisioningStates) error {
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
