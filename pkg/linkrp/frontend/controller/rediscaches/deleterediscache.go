// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	fctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteRedisCache)(nil)

// DeleteRedisCache is the controller implementation to delete rediscache link resource.
type DeleteRedisCache struct {
	ctrl.Operation[*datamodel.RedisCache, datamodel.RedisCache]
	dp deployment.DeploymentProcessor
}

// NewDeleteRedisCache creates a new instance DeleteRedisCache.
func NewDeleteRedisCache(opts fctrl.Options) (ctrl.Controller, error) {
	return &DeleteRedisCache{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RedisCache]{
				RequestConverter:  converter.RedisCacheDataModelFromVersioned,
				ResponseConverter: converter.RedisCacheDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

func (redis *DeleteRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	old, etag, err := redis.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if r, err := redis.PrepareResource(ctx, req, nil, old, etag); r != nil || err != nil {
		return r, err
	}

	err = redis.dp.Delete(ctx, deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: old, OutputResources: old.Properties.Status.OutputResources, ComputedValues: old.ComputedValues, SecretValues: old.SecretValues, RecipeData: old.RecipeData})
	if err != nil {
		return nil, err
	}

	if err := redis.StorageClient().Delete(ctx, serviceCtx.ResourceID.String()); err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
