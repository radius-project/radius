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
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/queue"

	"golang.org/x/sync/semaphore"
)

const (
	// MaxConcurrency is the maximum concurrency to process async request operation.
	MaxConcurrency = 5

	// MaxDequeueCount is the maximum dequeue count which will be retried.
	MaxDequeueCount = 3
)

type AsyncRequestProcessor struct {
	Options               hostoptions.HostOptions
	requestQueue          queue.Dequeuer
	sp                    dataprovider.DataStorageProvider
	handlers              *HandlerRegistry
	asyncOperationManager asyncoperation.AsyncOperationManagerInterface

	sem *semaphore.Weighted
}

func NewAsyncRequestProcessor(
	options hostoptions.HostOptions,
	sp dataprovider.DataStorageProvider,
	dequeueClient queue.Dequeuer,
	asyncOperationManager asyncoperation.AsyncOperationManagerInterface,
	handlers *HandlerRegistry) *AsyncRequestProcessor {
	return &AsyncRequestProcessor{
		Options:               options,
		sp:                    sp,
		requestQueue:          dequeueClient,
		handlers:              handlers,
		asyncOperationManager: asyncOperationManager,
		sem:                   semaphore.NewWeighted(MaxConcurrency),
	}
}

func (w *AsyncRequestProcessor) Start(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	ctx = hostoptions.WithContext(ctx, w.Options.Config)
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
			op := jobm.Data.(*datamodel.AsyncOperationMessage)

			ctrl := w.handlers.GetController(op.OperationName)
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

	logger.Info("Server stopped...")
	return nil
}

func (w *AsyncRequestProcessor) runOperation(ctx context.Context, asyncMessage *queue.Message, ctrl asyncctrl.AsyncControllerInterface) {
	logger := logr.FromContextOrDiscard(ctx)
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	op := asyncMessage.Data.(*datamodel.AsyncOperationMessage)

	go func() {
		// TODO: handle error from ctrl.Run()
		_ = ctrl.Run(ctx)
	}()

	for {
		select {
		case <-time.After(asyncMessage.NextVisibleAt.Sub(time.Now())):
			logger.Info("Extending message lock if operation is still in progress")
			asyncMessage.Extend()

		case resp := <-ctrl.AsyncResponseCh():
			logger.Info("Getting async response. Status:", resp.Status)
			switch resp.Status {
			case basedatamodel.ProvisioningStateCanceled, basedatamodel.ProvisioningStateFailed, basedatamodel.ProvisioningStateSucceeded:
				w.onCompleteOperation(ctx, resp, ctrl)
				return
			default:
				// TODO: Handle the other state properly.
			}

		case <-time.After(op.AsyncOperationTimeout):
			logger.Info("Timed out async request operation.")

			resp := asyncctrl.NewAsyncResponse(op.AsyncOperationID, op.OperationName, sCtx.ResourceID, basedatamodel.ProvisioningStateCanceled)
			resp.SetCanceled("async operation timeout")

			w.onCompleteOperation(ctx, resp, ctrl)
			return

		case <-ctx.Done():
			// Exiting worker
			return
		}
	}
}

func (w *AsyncRequestProcessor) onCompleteOperation(ctx context.Context, asyncResp *asyncctrl.AsyncResponse, ctrl asyncctrl.AsyncControllerInterface) {
	_ = servicecontext.ARMRequestContextFromContext(ctx)

	if asyncResp.Error != nil {
		// TODO: Update OperationStatuses with
	}

	// update resource provisioning state
	// finish message
}
