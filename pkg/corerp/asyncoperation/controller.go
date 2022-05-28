// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package asyncoperation

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/store"
)

// Controller is an interface to implement async operation controller. This is to implement request-reply pattern using messaging queue.
// Frontend Controller enqueues AsyncRequestMessage and creates OperationStatuses. AsyncRequestProcessWorker consumes this async request
// message and executes this AsyncController. To implement "Reply" pattern, it uses go channel and Worker listens to this reply go channel to
// update OperationStatuses record. AsyncController can use Reply() to send the response to worker over go channel.
type Controller interface {
	// Run runs async request operation.
	Run(ctx context.Context, request *AsyncRequestMessage) (Result, error)
	StorageClient() store.StorageClient
}

// BaseController is the base struct of async operation controller.
type BaseController struct {
	storageClient store.StorageClient
}

// NewBaseAsyncController creates BaseAsyncController instance.
func NewBaseAsyncController(store store.StorageClient) BaseController {
	return BaseController{storageClient: store}
}

// StorageClient gets storage client for this controller.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.storageClient
}
