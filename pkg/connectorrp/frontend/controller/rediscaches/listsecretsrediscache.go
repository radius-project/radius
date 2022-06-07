// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"net/http"

	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	base_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller"
	"github.com/project-radius/radius/pkg/corerp/servicecontext"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ base_ctrl.ControllerInterface = (*ListSecretsRedisCache)(nil)

// ListSecretsRedisCache is the controller implementation to list secrets for the to access the connected redis cache resource resource id passed in the request body.
type ListSecretsRedisCache struct {
	base_ctrl.BaseController
}

// NewListSecretsRedisCache creates a new instance of ListSecretsRedisCache.
func NewListSecretsRedisCache(storageClient store.StorageClient, jobEngine deployment.DeploymentProcessor) (base_ctrl.ControllerInterface, error) {
	return &ListSecretsRedisCache{
		BaseController: base_ctrl.BaseController{
			DBClient:  storageClient,
			JobEngine: jobEngine,
		},
	}, nil
}

// Run returns secrets values for the specified RedisCache resource
func (ctrl *ListSecretsRedisCache) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.RedisCache{}
	_, err := ctrl.GetResource(ctx, sCtx.ResourceID.String(), resource)
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
