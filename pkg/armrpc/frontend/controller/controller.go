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
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
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

	// AsyncOperation
	AsyncOperation sm.StatusManager
}

// Controller is an interface of each operation controller.
type Controller interface {
	// Run executes the operation.
	Run(ctx context.Context, req *http.Request) (rest.Response, error)
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

// DeploymentProcessor gets the deployment processor for this controller.
func (b *BaseController) StatusManager() sm.StatusManager {
	return b.options.AsyncOperation
}

// GetResource is the helper to get the resource via storage client.
func (c *BaseController) GetResource(ctx context.Context, id string, out interface{}) (etag string, err error) {
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
func (c *BaseController) SaveResource(ctx context.Context, id string, in interface{}, etag string) (*store.Object, error) {
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
	requestCtx := servicecontext.ARMRequestContextFromContext(ctx)
	serviceOpt := hostoptions.FromContext(ctx)

	trackedResource := v1.TrackedResource{
		ID:       requestCtx.ResourceID.String(),
		Name:     requestCtx.ResourceID.Name(),
		Type:     requestCtx.ResourceID.Type(),
		Location: serviceOpt.Env.RoleLocation,
	}

	return trackedResource
}
