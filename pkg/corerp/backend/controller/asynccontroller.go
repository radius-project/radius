// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/store"
)

// AsyncController is an interface to implement async operation controller.
type AsyncController interface {
	// Run runs async request operation.
	Run(ctx context.Context) error

	// AsyncResponseCh gets the output AsyncResponse channel. Worker will listen this channel to update operationstatus record.
	AsyncResponseCh() <-chan *AsyncReplyResponse
	// Reply stores async request response.
	Reply(resp *AsyncReplyResponse)

	// StorageClient gets storage client for this controller
	StorageClient() store.StorageClient
}

// BaseAsyncController is the base struct of async operation controller.
type BaseAsyncController struct {
	StoreClient store.StorageClient
	AsyncResp   chan *AsyncReplyResponse
}

// AsyncResponseCh gets the output channel of asynchronous response.
func (b *BaseAsyncController) AsyncResponseCh() <-chan *AsyncReplyResponse {
	return b.AsyncResp
}

// Reply replies the async response.
func (b *BaseAsyncController) Reply(resp *AsyncReplyResponse) {
	b.AsyncResp <- resp
}
