// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package rediscaches

import (
	"context"
	"errors"
	"net/http"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/connectorrp/renderers"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsRedisCache)(nil)

// ListSecretsRedisCache is the controller implementation to list secrets for the to access the connected redis cache resource resource id passed in the request body.
type ListSecretsRedisCache struct {
	ctrl.BaseController
}

// NewListSecretsRedisCache creates a new instance of ListSecretsRedisCache.
func NewListSecretsRedisCache(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsRedisCache{ctrl.NewBaseController(opts)}, nil
}

// Run returns secrets values for the specified RedisCache resource
func (ctrl *ListSecretsRedisCache) Run(ctx context.Context, req *http.Request) (rest.Response, error) {
	sCtx := servicecontext.ARMRequestContextFromContext(ctx)

	resource := &datamodel.RedisCache{}
	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Connector/redisCaches/<resource_name>/listsecrets
	parsedResourceID := sCtx.ResourceID.Truncate()
	_, err := ctrl.GetResource(ctx, parsedResourceID.String(), resource)
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	secrets, err := ctrl.DeploymentProcessor().FetchSecrets(ctx, deployment.ResourceData{ID: sCtx.ResourceID, Resource: resource, OutputResources: resource.Properties.Status.OutputResources, ComputedValues: resource.ComputedValues, SecretValues: resource.SecretValues})
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
