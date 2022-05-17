// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/jobqueue"

	"golang.org/x/sync/semaphore"
)

type AsyncRequestProcessor struct {
	Options       hostoptions.HostOptions
	dequeueClient jobqueue.Dequeuer
	sp            dataprovider.DataStorageProvider

	sem *semaphore.Weighted
}

func NewAsyncRequestProcessor(options hostoptions.HostOptions, sp dataprovider.DataStorageProvider, dequeueClient jobqueue.Dequeuer) *AsyncRequestProcessor {
	return &AsyncRequestProcessor{
		Options:       options,
		sp:            sp,
		dequeueClient: dequeueClient,
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
		go func(jr *jobqueue.JobMessageResponse) {
			defer w.sem.Release(1)
			w.getOperationController(ctx, jr)
		}(&job)
	}

	logger.Info("Server stopped...")
	return nil
}

func (w *AsyncRequestProcessor) getOperationController(ctx context.Context, job *jobqueue.JobMessageResponse) error {
	return nil
}

func (w *AsyncRequestProcessor) runOperation(ctx context.Context, job *jobqueue.JobMessageResponse) error {
	return nil
}
