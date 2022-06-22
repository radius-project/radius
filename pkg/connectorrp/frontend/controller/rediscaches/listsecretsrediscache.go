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
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsRedisCache)(nil)

// ListSecretsRedisCache is the controller implementation to list secrets for the to access the connected redis cache resource resource id passed in the request body.
type ListSecretsRedisCache struct {
	ctrl.BaseController
}

// NewListSecretsRedisCache creates a new instance of ListSecretsRedisCache.
func NewListSecretsRedisCache(ds store.StorageClient, sm manager.StatusManager) (ctrl.Controller, error) {
	return &ListSecretsRedisCache{ctrl.NewBaseController(ds, sm)}, nil
}

// Run returns secrets values for the specified RedisCache resource
func (ctrl *ListSecretsRedisCache) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.RedisCache{}
	parsedResourceID := sCtx.ResourceID.Truncate()
	_, err := ctrl.GetResource(ctx, parsedResourceID.String(), resource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	// TODO integrate with deploymentprocessor
	// output, err := ctrl.JobEngine.FetchSecrets(ctx, sCtx.ResourceID, resource)
	// if err != nil {
	// 	return nil, err
	// }

	versioned, _ := converter.RedisCacheSecretsDataModelToVersioned(&datamodel.RedisCacheSecrets{}, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
