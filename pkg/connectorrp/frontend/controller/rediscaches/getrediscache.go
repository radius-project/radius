// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"net/http"

	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*GetRedisCache)(nil)

// GetRedisCache is the controller implementation to get the redisCache conenctor resource.
type GetRedisCache struct {
	ctrl.BaseController
}

// NewGetRedisCache creates a new instance of GetRedisCache.
func NewGetRedisCache(ds store.StorageClient, sm manager.StatusManager, dp deployment.DeploymentProcessor) (ctrl.Controller, error) {
	return &GetRedisCache{ctrl.NewBaseController(ds, sm, dp)}, nil
}

func (redis *GetRedisCache) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	serviceCtx := servicecontext.ARMRequestContextFromContext(ctx)

	existingResource := &datamodel.RedisCacheResponse{}
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
