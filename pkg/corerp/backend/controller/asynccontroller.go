// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/datamodel/asyncoperation"
	"github.com/project-radius/radius/pkg/store"
)

// AsyncController is an interface to implement async operation controller. This is to implement request-reply pattern using messaging queue.
// Frontend Controller enqueues AsyncRequestMessage and creates OperationStatuses. AsyncRequestProcessWorker consumes this async request
// message and executes this AsyncController. To implement Reply pattern, it uses go channel and Worker listens to this reply go channel to
// update OperationStatuses record. AsyncController can use Reply() to send the response to worker over go channel.
type AsyncController interface {
	// Run runs async request operation.
	Run(ctx context.Context) error
	// Reply stores async request response.
	Reply(resp *asyncoperation.AsyncReplyResponse)

	// AsyncResponseCh gets the output AsyncResponse channel. Worker will listen this channel to update operationstatus record.
	AsyncResponseCh() <-chan *asyncoperation.AsyncReplyResponse
	// StorageClient gets storage client for this controller.
	StorageClient() store.StorageClient
}

// BaseAsyncController is the base struct of async operation controller.
type BaseAsyncController struct {
	storageClient store.StorageClient
	asyncRespCh   chan *asyncoperation.AsyncReplyResponse
}

// NewBaseAsyncController creates BaseAsyncController instance.
func NewBaseAsyncController(store store.StorageClient, ch chan *asyncoperation.AsyncReplyResponse) BaseAsyncController {
	return BaseAsyncController{storageClient: store, asyncRespCh: ch}
}

// StorageClient gets storage client for this controller.
func (b *BaseAsyncController) StorageClient() store.StorageClient {
	return b.storageClient
}

// AsyncResponseCh gets the output channel of asynchronous response.
func (b *BaseAsyncController) AsyncResponseCh() <-chan *asyncoperation.AsyncReplyResponse {
	return b.asyncRespCh
}

// Reply replies the async response.
func (b *BaseAsyncController) Reply(resp *asyncoperation.AsyncReplyResponse) {
	b.asyncRespCh <- resp
}
