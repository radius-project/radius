// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/store"
)

// Controller is an interface to implement async operation controller.
type Controller interface {
	// Run runs async request operation.
	Run(ctx context.Context, request *Request) (Result, error)

	// StorageClient gets the storage client for resource type.
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
