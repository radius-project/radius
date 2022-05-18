// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	asyncctrl "github.com/project-radius/radius/pkg/corerp/backend/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/jobqueue"

	"golang.org/x/sync/semaphore"
)

type AsyncRequestProcessor struct {
	Options       hostoptions.HostOptions
	dequeueClient jobqueue.Dequeuer
	sp            dataprovider.DataStorageProvider
	handlers      *HandlerRegistry

	sem *semaphore.Weighted
}

func NewAsyncRequestProcessor(options hostoptions.HostOptions, sp dataprovider.DataStorageProvider, dequeueClient jobqueue.Dequeuer, handlers *HandlerRegistry) *AsyncRequestProcessor {
	return &AsyncRequestProcessor{
		Options:       options,
		sp:            sp,
		dequeueClient: dequeueClient,
		handlers:      handlers,
		sem:           semaphore.NewWeighted(5),
	}
}

func (w *AsyncRequestProcessor) Start(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	ctx = hostoptions.WithContext(ctx, w.Options.Config)
	jobs, err := w.dequeueClient.Dequeue(ctx)
	if err != nil {
		return errors.New("fails to initialize job queue")
	}

	for job := range jobs {
		if err := w.sem.Acquire(ctx, 1); err != nil {
			break
		}

		go func(jobm *jobqueue.Message) {
			defer w.sem.Release(1)
			op := jobm.Data.(*datamodel.AsyncOperationMessage)

			fn := w.handlers.GetController(op.OperationName)
			if fn == nil {
				jobm.Finish(errors.New("unknown operation"))
				return
			}

			// TODO: convert op to armservicecontext and inject to ctx
			err := w.runOperation(ctx, fn)
			if err != nil {
				logger.Info("Failed operation: ", op.OperationName, ", Error: ", err)
			}

			// TODO: validate all failed conditions.
			if jobm.DequeueCount >= 5 {
				jobm.Finish(errors.New("too many retries"))
			}
		}(job)
	}

	logger.Info("Server stopped...")
	return nil
}

func (w *AsyncRequestProcessor) runOperation(ctx context.Context, ctrl asyncctrl.AsyncControllerInterface) error {

	return nil
}
