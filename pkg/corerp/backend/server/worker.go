// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"errors"
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/basedatamodel"
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/queue"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"

	"golang.org/x/sync/semaphore"
)

const (
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency = 3

	// MaxDequeueCount is the maximum dequeue count which will be retried.
	MaxDequeueCount = 3
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

			// TODO: convert op to armservicecontext and inject to ctx
			w.runOperation(ctx, jobm, ctrl)

			// TODO: validate all failed conditions.
			if jobm.DequeueCount >= MaxDequeueCount {
				jobm.Finish(errors.New("too many retries"))
			}
		}(job)
	}

	logger.Info("Message loop stopped...")
	return nil
}

func (w *AsyncRequestProcessWorker) runOperation(ctx context.Context, asyncMessage *queue.Message, ctrl asyncctrl.AsyncController) {
	logger := logr.FromContextOrDiscard(ctx)
	asyncReq := asyncMessage.Data.(*asyncoperation.AsyncRequestMessage)

	go func() {
		err := ctrl.Run(ctx, asyncReq)
		if err != nil {
		}

	}()

	for {
		select {
		case <-time.After(asyncMessage.NextVisibleAt.Sub(time.Now())):
			logger.Info("Extending message lock if operation is still in progress")
			if err := asyncMessage.Extend(); err != nil {
				// TODO: async message
			}

		case resp := <-ctrl.ResultCh():
			logger.Info("Getting async result.", "Status", resp.Status, "OperationID", resp.OperationID)
			switch resp.Status {
			case basedatamodel.ProvisioningStateCanceled, basedatamodel.ProvisioningStateFailed, basedatamodel.ProvisioningStateSucceeded:
				w.completeOperation(ctx, resp, ctrl.StorageClient())
				return
			default:
				// TODO: Handle the other state properly.
			}

		case <-time.After(asyncReq.AsyncOperationTimeout):
			logger.Info("Timed out async request operation.")

			rID, err := resources.Parse(asyncReq.ResourceID)
			if err != nil {
				logger.Error(err, "failed to parse resource ID", "resourceID", asyncReq.ResourceID)
			}

			resp := asyncoperation.NewAsyncOperationResult(asyncReq.OperationID, asyncReq.OperationName, rID, basedatamodel.ProvisioningStateCanceled)
			resp.SetCanceled("async operation timeout")

			w.completeOperation(ctx, resp, ctrl.StorageClient())
			return

		case <-ctx.Done():
			// Exiting worker
			return
		}
	}
}

func (w *AsyncRequestProcessWorker) completeOperation(ctx context.Context, result *asyncoperation.AsyncOperationResult, store store.StorageClient) {
	if result.Error != nil {
		// TODO: Update OperationStatuses with
	}

	// update resource provisioning state
	// finish message
}
