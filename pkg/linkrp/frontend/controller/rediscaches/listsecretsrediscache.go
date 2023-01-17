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
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsRedisCache)(nil)

// ListSecretsRedisCache is the controller implementation to list secrets for the to access the connected redis cache resource resource id passed in the request body.
type ListSecretsRedisCache struct {
	ctrl.Operation[*datamodel.RedisCache, datamodel.RedisCache]
	dp deployment.DeploymentProcessor
}

// NewListSecretsRedisCache creates a new instance of ListSecretsRedisCache.
func NewListSecretsRedisCache(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsRedisCache{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.RedisCache]{
				RequestConverter:  converter.RedisCacheDataModelFromVersioned,
				ResponseConverter: converter.RedisCacheDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

// Run returns secrets values for the specified RedisCache resource
func (ctrl *ListSecretsRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Link/redisCaches/<resource_name>/listsecrets
	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	secrets, err := ctrl.dp.FetchSecrets(ctx, deployment.ResourceData{ID: sCtx.ResourceID, Resource: resource, OutputResources: resource.Properties.Status.OutputResources, ComputedValues: resource.ComputedValues, SecretValues: resource.SecretValues})
	if err != nil {
		return nil, err
	}

	redisSecrets := datamodel.RedisCacheSecrets{}
	if password, ok := secrets[renderers.PasswordStringHolder].(string); ok {
		redisSecrets.Password = password
	}
	if connectionString, ok := secrets[renderers.ConnectionStringValue].(string); ok {
		redisSecrets.ConnectionString = connectionString
	}

	versioned, _ := converter.RedisCacheSecretsDataModelToVersioned(&redisSecrets, sCtx.APIVersion)
	return rest.NewOKResponse(versioned), nil
}
