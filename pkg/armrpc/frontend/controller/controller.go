/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	sm "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	sv "github.com/project-radius/radius/pkg/rp/secretvalue"
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
	SecretClient sv.SecretValueClient

	// KubeClient is the Kubernetes controller runtime client.
	KubeClient runtimeclient.Client

	// ResourceType is the string that represents the resource type.
	ResourceType string

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

	// AsyncOperationTimeout is the default timeout duration of async put operation.
	AsyncOperationTimeout time.Duration

	// AsyncOperationRetryAfter is the value of the Retry-After header that will be used for async operations.
	// If this is 0 then the default value of v1.DefaultRetryAfter will be used. Consider setting this to a smaller
	// value like 5 seconds if your operations will complete quickly.
	AsyncOperationRetryAfter time.Duration
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
//
// # Function Explanation
// 
//	NewBaseController creates a new BaseController object with the given options and returns it. If any errors occur, they 
//	will be returned to the caller.
func NewBaseController(options Options) BaseController {
	return BaseController{
		options,
	}
}

// StorageClient gets storage client for this controller.
//
// # Function Explanation
// 
//	BaseController's StorageClient function returns the StorageClient option from the BaseController's options struct, 
//	allowing callers to access the StorageClient. If the StorageClient option is not set, an error is returned.
func (b *BaseController) StorageClient() store.StorageClient {
	return b.options.StorageClient
}

// DataProvider gets data storage provider for this controller.
//
// # Function Explanation
// 
//	BaseController's DataProvider() function returns the DataStorageProvider from the options struct, allowing callers to 
//	access the data provider. If the data provider is not set, an error is returned.
func (b *BaseController) DataProvider() dataprovider.DataStorageProvider {
	return b.options.DataProvider
}

// SecretClient gets secret client for this controller.
//
// # Function Explanation
// 
//	The SecretClient() function returns a SecretValueClient from the options provided, and handles any errors that may occur
//	 during the process.
func (b *BaseController) SecretClient() sv.SecretValueClient {
	return b.options.SecretClient
}

// KubeClient gets Kubernetes client for this controller.
//
// # Function Explanation
// 
//	The BaseController.KubeClient() function returns a runtimeclient.Client object which is used to interact with the 
//	Kubernetes API. It handles any errors that occur during the process and returns an error if one is encountered.
func (b *BaseController) KubeClient() runtimeclient.Client {
	return b.options.KubeClient
}

// ResourceType gets the resource type for this controller.
//
// # Function Explanation
// 
//	BaseController's ResourceType function returns the resource type of the controller, or an error if the resource type is 
//	not set.
func (b *BaseController) ResourceType() string {
	return b.options.ResourceType
}

// DeploymentProcessor gets the deployment processor for this controller.
//
// # Function Explanation
// 
//	The StatusManager() function returns the StatusManager object from the options struct, allowing callers to access the 
//	status manager and handle errors accordingly.
func (b *BaseController) StatusManager() sm.StatusManager {
	return b.options.StatusManager
}

// GetResource is the helper to get the resource via storage client.
//
// # Function Explanation
// 
//	GetResource retrieves an object from the storage client and attempts to convert it into the given output type. It 
//	returns the ETag of the object and any errors encountered. If an error is encountered, the output will be nil.
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
//
// # Function Explanation
// 
//	SaveResource saves a resource to the storage client, using the given ID and input data, and an optional ETag. It returns
//	 the saved object or an error if the save fails.
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
//
// # Function Explanation
// 
//	UpdateSystemData updates the old SystemData with the new SystemData, backfilling the CreatedAt, CreatedBy, and 
//	CreatedByType fields if they are not set in the old SystemData. If LastModifiedAt is set in the new SystemData, it will 
//	update the LastModifiedAt, LastModifiedBy, and LastModifiedByType fields in the old SystemData.
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
//
// # Function Explanation
// 
//	BuildTrackedResource extracts information from the context and request object to create a TrackedResource object. It 
//	handles errors by returning an empty TrackedResource if the context or request object is invalid.
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
