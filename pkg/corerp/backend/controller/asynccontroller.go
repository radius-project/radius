// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/store"
)

// AsyncControllerInterface is an interface to implement async operation controller.
type AsyncControllerInterface interface {
	// Run runs async request operation.
	Run(ctx context.Context) error

	// AsyncResponseCh gets the output AsyncResponse channel.
	AsyncResponseCh() <-chan *AsyncResponse
	// Reply stores async request response.
	Reply(resp *AsyncResponse)

	// StorageClient gets storage client for this controller
	StorageClient() store.StorageClient
}

// BaseAsyncController is the base struct of async operation controller.
type BaseAsyncController struct {
	StoreClient store.StorageClient
	AsyncResp   chan *AsyncResponse
}

// AsyncResponseCh gets the output channel of asynchronous response.
func (b *BaseAsyncController) AsyncResponseCh() <-chan *AsyncResponse {
	return b.AsyncResp
}

// Reply replies the async response.
func (b *BaseAsyncController) Reply(resp *AsyncResponse) {
	b.AsyncResp <- resp
}
