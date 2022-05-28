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
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/ucp/store"

	"golang.org/x/sync/semaphore"
)

const (
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency = 3

	// MaxDequeueCount is the maximum dequeue count which will be retried.
	MaxDequeueCount = 10
)

// AsyncRequestProcessWorker is the worker to process async requests.
type AsyncRequestProcessWorker struct {
	options      hostoptions.HostOptions
	sp           dataprovider.DataStorageProvider
	controllers  *ControllerRegistry
	requestQueue queue.Dequeuer

	sem *semaphore.Weighted
}

// NewAsyncRequestProcessWorker creates AsyncRequestProcessWorker server instance.
func NewAsyncRequestProcessWorker(
	options hostoptions.HostOptions,
	sp dataprovider.DataStorageProvider,
	ctrlRegistry *ControllerRegistry) *AsyncRequestProcessWorker {
	return &AsyncRequestProcessWorker{
		options:     options,
		sp:          sp,
		controllers: ctrlRegistry,

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

			op := jobm.Data.(*asyncoperation.AsyncRequestMessage)

			ctrl := w.controllers.Get(op.OperationName)
			if ctrl == nil {
				jobm.Finish(errors.New("unknown operation"))
				return
			}

			// TODO: validate all failed conditions.
			if jobm.DequeueCount >= MaxDequeueCount {
				jobm.Finish(errors.New("too many retries"))
				return
			}

			// TODO: convert op to armservicecontext and inject to ctx
			w.runOperation(ctx, jobm, ctrl)
		}(job)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, message *queue.Message, ctrl asyncoperation.Controller) {
	logger := logr.FromContextOrDiscard(ctx)
	asyncReq := message.Data.(*asyncoperation.AsyncRequestMessage)
	//rID, err := resources.Parse(asyncReq.ResourceID)
	//if err != nil {
	//	logger.Error(err, "failed to parse resource ID", "resourceID", asyncReq.ResourceID)
	//}

	asyncReqCtx, opCancel := context.WithCancel(context.TODO())
	defer opCancel()

	go func() {
		defer func() {
			if err := recover(); err != nil {
				msg := fmt.Sprintf("recovering from panic %v: %s", err, debug.Stack())
				logger.V(radlogger.Fatal).Info(msg)

				result := asyncoperation.Result{}
				result.SetFailed(armerrors.ErrorDetails{Code: armerrors.Internal, Message: "unexpected error."})

				w.completeOperation(ctx, asyncReq, &result, ctrl.StorageClient())
			}
		}()

		result, err := ctrl.Run(asyncReqCtx, asyncReq)
		if err != nil {
			result.SetFailed(armerrors.ErrorDetails{Code: armerrors.Internal, Message: err.Error()})
		}
		w.completeOperation(ctx, asyncReq, &result, ctrl.StorageClient())
	}()

	for {
		select {
		case <-time.After(message.NextVisibleAt.Sub(time.Now())):
			logger.Info("Extending message lock if operation is still in progress")
			if err := message.Extend(); err != nil {
				logger.Error(err, "fails to extend message lock", "OperationID", asyncReq.OperationID.String())
			}

		case <-time.After(asyncReq.AsyncOperationTimeout):
			logger.Info("Timed out async request operation.")

			opCancel()

			result := &asyncoperation.Result{}
			result.SetCanceled("request operation timed out")

			w.completeOperation(ctx, asyncReq, result, ctrl.StorageClient())
			return

		case <-ctx.Done():
			opCancel()

			result := &asyncoperation.Result{}
			result.SetCanceled("request operation timed out")

			w.completeOperation(ctx, asyncReq, result, ctrl.StorageClient())
			return
		}
	}
}

func (w *AsyncRequestProcessWorker) completeOperation(ctx context.Context, req *asyncoperation.AsyncRequestMessage, result *asyncoperation.Result, store store.StorageClient) {
	if result.Error != nil {
		// TODO: Update OperationStatuses with
	}

	// update resource provisioning state
	// finish message
}
