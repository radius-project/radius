// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	sm "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/rp"
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
	SecretClient rp.SecretValueClient

	// KubeClient is the Kubernetes controller runtime client.
	KubeClient runtimeclient.Client

	// ResourceType is the string that represents the resource type.
	ResourceType string

	// GetDeploymentProcessor is the factory function to create DeploymentProcessor instance.
	GetDeploymentProcessor func() deployment.DeploymentProcessor

	// StatusManager
	StatusManager sm.StatusManager
}

// ResourceOptions represents the options and filters for resource.
type ResourceOptions[T any] struct {
	// RequestConverter is the request converter.
	RequestConverter v1.ConvertToDataModel[T]

	// ResponseConverter is the response converter.
	ResponseConverter v1.ConvertToAPIModel[T]

	// DeleteFilters is a slice of filters that execute prior to deleting a resource.
	DeleteFilters []DeleteFilter[T]

	// UpdateFilters is a slice of filters that execute prior to updating a resource.
	UpdateFilters []UpdateFilter[T]
}

// TODO: Remove Controller when all controller uses Operation
// Controller is an interface of each operation controller.
type Controller interface {
	// Run executes the operation.
	Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error)
}

// BaseController is the base operation controller.
type BaseController struct {
	options Options
}

// NewBaseController creates BaseController instance.
func NewBaseController(options Options) BaseController {
	return BaseController{
		options,
	}
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
func (b *BaseController) SecretClient() rp.SecretValueClient {
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

// DeploymentProcessor gets the deployment processor for this controller.
func (b *BaseController) StatusManager() sm.StatusManager {
	return b.options.StatusManager
}

// GetResource is the helper to get the resource via storage client.
func (c *BaseController) GetResource(ctx context.Context, id string, out any) (etag string, err error) {
	etag = ""
	var res *store.Object
	if res, err = c.StorageClient().Get(ctx, id); err == nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}

// SaveResource is the helper to save the resource via storage client.
func (c *BaseController) SaveResource(ctx context.Context, id string, in any, etag string) (*store.Object, error) {
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := c.StorageClient().Save(ctx, nr, store.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}

// UpdateSystemData creates or updates new systemdata from old and new resources.
func UpdateSystemData(old v1.SystemData, new v1.SystemData) v1.SystemData {
	newSystemData := old

	if old.CreatedAt == "" && new.CreatedAt != "" {
		newSystemData.CreatedAt = new.CreatedAt
		newSystemData.CreatedBy = new.CreatedBy
		newSystemData.CreatedByType = new.CreatedByType
	}

	if new.LastModifiedAt != "" {
		newSystemData.LastModifiedAt = new.LastModifiedAt
		newSystemData.LastModifiedBy = new.LastModifiedBy
		newSystemData.LastModifiedByType = new.LastModifiedByType

		// backfill
		if newSystemData.CreatedAt == "" {
			newSystemData.CreatedAt = new.LastModifiedAt
			newSystemData.CreatedBy = new.LastModifiedBy
			newSystemData.CreatedByType = new.LastModifiedByType
		}
	}

	return newSystemData
}

// BuildTrackedResource create TrackedResource instance from request context
func BuildTrackedResource(ctx context.Context) v1.TrackedResource {
	requestCtx := v1.ARMRequestContextFromContext(ctx)
	serviceOpt := hostoptions.FromContext(ctx)

	trackedResource := v1.TrackedResource{
		ID:       requestCtx.ResourceID.String(),
		Name:     requestCtx.ResourceID.Name(),
		Type:     requestCtx.ResourceID.Type(),
		Location: serviceOpt.Env.RoleLocation,
	}

	return trackedResource
}

// DeleteFilter is a function that is executed as part of the controller lifecycle. DeleteFilters can be used to:
//
// - Block deletion of a resource based on some arbitrary condition.
//
// DeleteFilters should return a rest.Response to handle the request without allowing deletion to occur. Any
// errors returned will be treated as "unhandled" and logged before sending back an HTTP 500.
type DeleteFilter[T any] func(ctx context.Context, oldResource *T, options *Options) (rest.Response, error)

// UpdateFilter is a function that is executed as part of the controller lifecycle. UpdateFilters can be used to:
//
// - Set internal state of a resource data model prior to saving.
// - Perform semantic validation based on the old state of a resource.
// - Perform semantic validation based on external state.
//
// UpdateFilters should return a rest.Response to handle the request without allowing updates to occur. Any
// errors returned will be treated as "unhandled" and logged before sending back an HTTP 500.
type UpdateFilter[T any] func(ctx context.Context, newResource *T, oldResource *T, options *Options) (rest.Response, error)
