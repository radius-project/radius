// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"net/http"
	"time"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
)

var _ ctrl.Controller = (*CreateOrUpdateRedisCache)(nil)

var (
	// AsyncPutContainerOperationTimeout is the default timeout duration of async put redisCache operation.
	AsyncPutContainerOperationTimeout = time.Duration(5) * time.Minute
)

// CreateOrUpdateRedisCache is the controller implementation to create or update RedisCache link resource.
type CreateOrUpdateRedisCache struct {
	ctrl.Operation[*datamodel.RedisCache, datamodel.RedisCache]
	dp deployment.DeploymentProcessor
}

// NewCreateOrUpdateRedisCache creates a new instance of CreateOrUpdateRedisCache.
func NewCreateOrUpdateRedisCache(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateRedisCache{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RedisCache]{
				RequestConverter:  converter.RedisCacheDataModelFromVersioned,
				ResponseConverter: converter.RedisCacheDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run executes CreateOrUpdateRedisCache operation.
func (redisCache *CreateOrUpdateRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	newResource, err := redisCache.GetResourceFromRequest(ctx, req)
	if err != nil {
		return nil, err
	}

	old, etag, err := redisCache.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	r, err := redisCache.PrepareResource(ctx, req, newResource, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	r, err = rp_frontend.PrepareRadiusResource(ctx, newResource, old, redisCache.Options())
	if r != nil || err != nil {
		return r, err
	}

	if r, err := redisCache.PrepareAsyncOperation(ctx, newResource, v1.ProvisioningStateAccepted, AsyncPutContainerOperationTimeout, &etag); r != nil || err != nil {
		return r, err

	}
	return redisCache.ConstructAsyncResponse(ctx, req.Method, etag, newResource)
}
