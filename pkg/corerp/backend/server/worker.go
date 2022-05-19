// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"

	"golang.org/x/sync/semaphore"
)

const (
	// MaxOperationConcurrency is the maximum concurrency to process async request operation.
	MaxOperationConcurrency = 3
)

// AsyncRequestProcessWorker is the worker to process async requests.
type AsyncRequestProcessWorker struct {
	options     hostoptions.HostOptions
	sp          dataprovider.DataStorageProvider
	controllers *ControllerRegistry
	sem         *semaphore.Weighted
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

	// TODO: implement message loop to run multiple operation concurrently.

	logger.Info("Server stopped...")
	return nil
}
