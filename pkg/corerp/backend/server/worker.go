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
	operationMgr asyncoperation.Manager
	registry     *ControllerRegistry
	requestQueue queue.Dequeuer

	sem *semaphore.Weighted
}

// NewAsyncRequestProcessWorker creates AsyncRequestProcessWorker server instance.
func NewAsyncRequestProcessWorker(
	options hostoptions.HostOptions,
	om asyncoperation.Manager,
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
	jobs, err := w.requestQueue.Dequeue(ctx)
	if err != nil {
		return errors.New("fails to initialize job queue")
	}

	for job := range jobs {
		if err := w.sem.Acquire(ctx, 1); err != nil {
			break
		}

		go func(jobm *queue.Message) {
			defer w.sem.Release(1)

			op := jobm.Data.(*asyncoperation.Request)
			ctrl := w.registry.Get(op.OperationName)

			dims := []interface{}{
				"OperationID", op.OperationID.String(),
				"OperationName", op.OperationName,
				"ResourceID", op.ResourceID,
				"CorrleationID", op.CorrelationID,
				"W3CTraceID", op.TraceparentID,
			}

			if ctrl == nil {
				logger.V(radlogger.Error).Info("Unknown operation: "+op.OperationName, dims...)
				if err := jobm.Finish(nil); err != nil {
					logger.V(radlogger.Error).Info("failed to finish the message which includes unknown operation.")
				}
				return
			}
			if jobm.DequeueCount >= MaxDequeueCount {
				logger.V(radlogger.Error).Info(fmt.Sprintf("Exceed max retrycount: %d", jobm.DequeueCount), dims...)
				if err := jobm.Finish(nil); err != nil {
					logger.V(radlogger.Error).Info("failed to finish the message which exceeds the max retry count.")
				}
				return
			}

			opCtx := logr.NewContext(ctx, logger.WithValues(dims...))
			w.runOperation(opCtx, jobm, ctrl)
		}(job)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, ctrl asyncoperation.Controller) {
	logger := logr.FromContextOrDiscard(ctx)

	asyncReq := message.Data.(*asyncoperation.Request)
	asyncReqCtx, opCancel := context.WithCancel(context.TODO())
	defer opCancel()

	opDone := make(chan struct{}, 1)

	opStartAt := time.Now()
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
			logger.Info("Extending message lock duration if operation is still in progress.")
			if err := message.Extend(); err != nil {
				logger.Error(err, "fails to extend message lock")
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

	s := &asyncoperation.Status{}
	s.Status = result.ProvisioningState()
	now := time.Now().UTC()
	s.EndTime = &now

	err = w.operationMgr.Update(ctx, rID.RootScope(), req.OperationID, s)
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
	d := visibleAt.Add(-messageExtendMargin).Sub(time.Now())
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

	if status, ok := objmap["provisioningState"].(string); !ok {
		return errProvisioningStateNotFound
	} else if status == string(state) {
		return nil
	}

	objmap["provisioningState"] = string(state)

	err = sc.Save(ctx, obj, store.WithETag(obj.ETag))
	if err != nil {
		return err
	}

	return nil
}
