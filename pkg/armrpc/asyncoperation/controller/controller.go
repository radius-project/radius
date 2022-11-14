// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"

	"github.com/project-radius/radius/pkg/corerp/backend/deployment"
	link_dp "github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/store"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type OptionsClassification interface {
	GetOptions() *Options
}

func (o *Options) GetOptions() *Options { return o }

// Options represents controller options.
type Options struct {
	// StorageClient is the data storage client.
	StorageClient store.StorageClient

	// DataProvider is the data storage provider.
	DataProvider dataprovider.DataStorageProvider

	// SecretClient is the client to fetch secrets.
	SecretClient rp.SecretValueClient

	// KubeClient is the Kubernetes controller runtime client.
	KubeClient runtimeclient.Client

	// ResourceType is the string that represents the resource type.
	ResourceType string
}
type CoreOptions struct {
	Options

	// GetDeploymentProcessor is the factory function to create core rp DeploymentProcessor instance.
	GetDeploymentProcessor func() deployment.DeploymentProcessor
}

func (r CoreOptions) GetOptions() *Options {
	return &Options{
		StorageClient: r.StorageClient,
		DataProvider:  r.DataProvider,
		SecretClient:  r.SecretClient,
		KubeClient:    r.KubeClient,
		ResourceType:  r.ResourceType,
	}
}

type LinkOptions struct {
	Options

	// GetLinkDeploymentProcessor is the factory function to create link rp DeploymentProcessor instance.
	GetDeploymentProcessor func() link_dp.DeploymentProcessor
}

func (r LinkOptions) GetOptions() *Options {
	return &Options{
		StorageClient: r.StorageClient,
		DataProvider:  r.DataProvider,
		SecretClient:  r.SecretClient,
		KubeClient:    r.KubeClient,
		ResourceType:  r.ResourceType,
	}
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
	options OptionsClassification
}

// NewBaseAsyncController creates BaseAsyncController instance.
func NewBaseAsyncController(options OptionsClassification) BaseController {
	return BaseController{options}
}

// StorageClient gets storage client for this controller.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.options.GetOptions().StorageClient
}

// DataProvider gets data storage provider for this controller.
func (b *BaseController) DataProvider() dataprovider.DataStorageProvider {
	return b.options.GetOptions().DataProvider
}

// SecretClient gets secret client for this controller.
func (b *BaseController) SecretClient() rp.SecretValueClient {
	return b.options.GetOptions().SecretClient
}

// KubeClient gets Kubernetes client for this controller.
func (b *BaseController) KubeClient() runtimeclient.Client {
	return b.options.GetOptions().KubeClient
}

// ResourceType gets the resource type for this controller.
func (b *BaseController) ResourceType() string {
	return b.options.GetOptions().ResourceType
}

// DeploymentProcessor gets the core rp deployment processor for this controller.
func (b *BaseController) DeploymentProcessor() interface{} {
	switch v := b.options.(type) {
	case CoreOptions:
		return v.GetDeploymentProcessor()
	case LinkOptions:
		return v.GetDeploymentProcessor()
	}
	return nil
}
