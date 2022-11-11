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
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetRedisCache)(nil)

// GetRedisCache is the controller implementation to get the redisCache conenctor resource.
type GetRedisCache struct {
	ctrl.BaseController
}

// NewGetRedisCache creates a new instance of GetRedisCache.
func NewGetRedisCache(opts ctrl.Options) (ctrl.Controller, error) {
	return &GetRedisCache{ctrl.NewBaseController(opts)}, nil
}

func (redis *GetRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.RedisCache{}
	_, err := redis.GetResource(ctx, serviceCtx.ResourceID.String(), existingResource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(serviceCtx.ResourceID), nil
		}
		return nil, err
	}

	versioned, _ := converter.RedisCacheDataModelToVersioned(existingResource, serviceCtx.APIVersion, false)
	return rest.NewOKResponse(versioned), nil
}
