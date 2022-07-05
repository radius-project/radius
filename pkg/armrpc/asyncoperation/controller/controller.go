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
	// StorageClient is the data storage client.
	StorageClient store.StorageClient

	// DataProvider is the data storage provider.
	DataProvider dataprovider.DataStorageProvider

	// SecretClient is the client to fetch secrets.
	SecretClient renderers.SecretValueClient

	// KubeClient is the Kubernetes controller runtime client.
	KubeClient runtimeclient.Client

	// ResourceType is the string that represents the resource type.
	ResourceType string

	// GetDeploymentProcessor is the factory function to create DeploymentProcessor instance.
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
	options Options
}

// NewBaseAsyncController creates BaseAsyncController instance.
func NewBaseAsyncController(options Options) BaseController {
	return BaseController{options}
}

// StorageClient gets storage client for this controller.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.options.StorageClient
}

// DataProvider gets data storage provider for this controller.
func (b *BaseController) DataProvider() dataprovider.DataStorageProvider {
	return b.options.DataProvider
}

// SecretClient gets secret client for this controller.
func (b *BaseController) SecretClient() renderers.SecretValueClient {
	return b.options.SecretClient
}

// KubeClient gets Kubernetes client for this controller.
func (b *BaseController) KubeClient() runtimeclient.Client {
	return b.options.KubeClient
}

// ResourceType gets the resource type for this controller.
func (b *BaseController) ResourceType() string {
	return b.options.ResourceType
}

// DeploymentProcessor gets the deployment processor for this controller.
func (b *BaseController) DeploymentProcessor() deployment.DeploymentProcessor {
	return b.options.GetDeploymentProcessor()
}

// GetResource is the helper to get the resource via storage client.
func (b *BaseController) GetResource(ctx context.Context, id string, out interface{}) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = b.StorageClient().Get(ctx, id); err == nil {
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
	err := b.StorageClient().Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}
