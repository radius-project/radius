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
	"errors"
	"net/http"
	"time"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	sm "github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/components/database"

	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Options represents controller options.
type Options struct {
	// Address is the listening address where the controller is running, including the hostname and port.
	//
	// For example: "localhost:8080".
	//
	// The listening address is provided so that it can be used when constructing URLs.
	Address string

	// PathBase is a URL path prefix that is applied to all requests and should not be considered part of request path
	// for determining routing or parsing of IDs. It must start with a slash or be empty.
	//
	// For example consider the following examples that match the resource ID "/planes/radius/local":
	//
	// - base path: "/apis/api.ucp.dev/v1alpha3" and URL path: "/apis/api.ucp.dev/planes/radius/local".
	// - base path: "" (empty) and request path: "/planes/radius/local".
	//
	// Code that needs to process the URL path should ignore the base path prefix when parsing the URL path.
	// Code that needs to construct a URL path should use the base path prefix when constructing the URL path.
	PathBase string

	// DatabaseClient is the database client.
	DatabaseClient database.Client

	// KubeClient is the Kubernetes controller runtime client.
	KubeClient runtimeclient.Client

	// ResourceType is the string that represents the resource type. May be empty if the controller
	// does not represent a single type of resource.
	ResourceType string

	// StatusManager is the async operation status manager.
	StatusManager sm.StatusManager
}

func (o Options) Validate() error {
	var err error
	if o.Address == "" {
		err = errors.Join(err, errors.New(".Address is required"))
	}
	if o.DatabaseClient == nil {
		err = errors.Join(err, errors.New(".DatabaseClient is required"))
	}
	if o.ResourceType == "" {
		err = errors.Join(err, errors.New(".ResourceType is required"))
	}
	if o.StatusManager == nil {
		err = errors.Join(err, errors.New(".StatusManager is required"))
	}

	// PathBase is usually empty, so it is not validated here.
	//
	// KubeClient is not used by the majority of the code, so it is not validated here.

	return err
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

	// ListRecursiveQuery specifies whether store query should be recursive or not. This should be set to true when the
	// scope of the list operation does not match the scope of the underlying resource type.
	//
	// This is ignored by non-list controllers.
	ListRecursiveQuery bool
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

// DatabaseClient gets database client for this controller.
func (b *BaseController) DatabaseClient() database.Client {
	return b.options.DatabaseClient
}

// KubeClient gets Kubernetes client for this controller.
func (b *BaseController) KubeClient() runtimeclient.Client {
	return b.options.KubeClient
}

// ResourceType gets the resource type for this controller.
func (b *BaseController) ResourceType() string {
	return b.options.ResourceType
}

// StatusManager gets the StatusManager of this controller.
func (b *BaseController) StatusManager() sm.StatusManager {
	return b.options.StatusManager
}

// GetResource gets a resource from data store for id, set the retrieved resource to out argument and returns
// the ETag of the resource and an error if one occurs.
func (c *BaseController) GetResource(ctx context.Context, id string, out any) (etag string, err error) {
	etag = ""
	var res *database.Object
	if res, err = c.DatabaseClient().Get(ctx, id); err == nil {
		if err = res.As(out); err == nil {
			etag = res.ETag
			return
		}
	}
	return
}

// SaveResource saves a resource to the data store with an ETag and returns a store object or an error if the save fails.
func (c *BaseController) SaveResource(ctx context.Context, id string, in any, etag string) (*database.Object, error) {
	nr := &database.Object{
		Metadata: database.Metadata{
			ID: id,
		},
		Data: in,
	}
	err := c.DatabaseClient().Save(ctx, nr, database.WithETag(etag))
	if err != nil {
		return nil, err
	}
	return nr, nil
}

// UpdateSystemData updates the system data fields in the old object with the new object's fields, backfilling the created
// fields if necessary.
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

// BuildTrackedResource takes in a context and returns a v1.TrackedResource object with the ID, Name, Type and Location
// fields populated from the context.
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
