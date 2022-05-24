// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/ucp/store"
)

// AsyncController is an interface to implement async operation controller. This is to implement request-reply pattern using messaging queue.
// Frontend Controller enqueues AsyncRequestMessage and creates OperationStatuses. AsyncRequestProcessWorker consumes this async request
// message and executes this AsyncController. To implement "Reply" pattern, it uses go channel and Worker listens to this reply go channel to
// update OperationStatuses record. AsyncController can use Reply() to send the response to worker over go channel.
type AsyncController interface {
	// Run runs async request operation.
	Run(ctx context.Context, message *asyncoperation.AsyncRequestMessage) error
	// Reply stores async request result.
	Reply(resp *asyncoperation.AsyncOperationResult)

	// ResultCh gets the output AsyncResponse channel. Worker will listen this channel to update operationstatus record.
	ResultCh() <-chan *asyncoperation.AsyncOperationResult
	// StorageClient gets storage client for this controller.
	StorageClient() store.StorageClient
}

// BaseAsyncController is the base struct of async operation controller.
type BaseAsyncController struct {
	storageClient store.StorageClient
	resultCh      chan *asyncoperation.AsyncOperationResult
}

// NewBaseAsyncController creates BaseAsyncController instance.
func NewBaseAsyncController(store store.StorageClient, ch chan *asyncoperation.AsyncOperationResult) BaseAsyncController {
	return BaseAsyncController{storageClient: store, resultCh: ch}
}

// StorageClient gets storage client for this controller.
func (b *BaseAsyncController) StorageClient() store.StorageClient {
	return b.storageClient
}

// ResultCh gets the output channel of asynchronous response.
func (b *BaseAsyncController) ResultCh() <-chan *asyncoperation.AsyncOperationResult {
	return b.resultCh
}

// Reply sends the result to resultCh. Controller can update the operationstatuses
// while processing async operation. For instance, it can set the status to Canceling
// during operation cancellation.
func (b *BaseAsyncController) Reply(resp *asyncoperation.AsyncOperationResult) {
	b.resultCh <- resp
}
