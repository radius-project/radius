// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	"github.com/project-radius/radius/pkg/renderers"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Options represents controller options.
type Options struct {
	StorageClient store.StorageClient
	DataProvider  dataprovider.DataStorageProvider

	SecretClient renderers.SecretValueClient // should be shared client
	KubeClient   runtimeclient.Client

	GetDeploymentProcessor func() deployment.DeploymentProcessor
}

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
func NewBaseAsyncController(options Options) BaseController {
	return BaseController{storageClient: options.StorageClient}
}

// StorageClient gets storage client for this controller.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.storageClient
}

// GetResource is the helper to get the resource via storage client.
func (b *BaseController) GetResource(ctx context.Context, id string, out interface{}) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = b.storageClient.Get(ctx, id); err == nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}

// SaveResource is the helper to save the resource via storage client.
func (b *BaseController) SaveResource(ctx context.Context, id string, in interface{}, etag string) (*store.Object, error) {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := b.storageClient.Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}
