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

package rediscaches

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	"github.com/radius-project/radius/pkg/datastoresrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/portableresources/renderers"
	"github.com/radius-project/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*ListSecretsRedisCache)(nil)

// ListSecretsRedisCache is the controller implementation to list secrets for the to access the connected Redis cache resource resource id passed in the request body.
type ListSecretsRedisCache struct {
	ctrl.Operation[*datamodel.RedisCache, datamodel.RedisCache]
}

// NewListSecretsRedisCache creates a new instance of ListSecretsRedisCache and returns it without an error.
func NewListSecretsRedisCache(opts ctrl.Options) (ctrl.Controller, error) {
	return &ListSecretsRedisCache{
		Operation: ctrl.NewOperation(opts,
			ctrl.ResourceOptions[datamodel.RedisCache]{
				RequestConverter:  converter.RedisCacheDataModelFromVersioned,
				ResponseConverter: converter.RedisCacheDataModelToVersioned,
			}),
	}, nil
}

// Run returns secrets values for the specified Redis cache resource
func (ctrl *ListSecretsRedisCache) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	sCtx := v1.ARMRequestContextFromContext(ctx)

	// Request route for listsecrets has name of the operation as suffix which should be removed to get the resource id.
	// route id format: subscriptions/<subscription_id>/resourceGroups/<resource_group>/providers/Applications.Datastores/redisCaches/<resource_name>/listsecrets
	parsedResourceID := sCtx.ResourceID.Truncate()
	resource, _, err := ctrl.GetResource(ctx, parsedResourceID)
	if err != nil {
		if errors.Is(&store.ErrNotFound{ID: parsedResourceID.String()}, err) {
			return rest.NewNotFoundResponse(sCtx.ResourceID), nil
		}
		return nil, err
	}

	if resource == nil {
		return rest.NewNotFoundResponse(sCtx.ResourceID), nil
	}

	redisSecrets := datamodel.RedisCacheSecrets{}
	if password, ok := resource.SecretValues[renderers.PasswordStringHolder]; ok {
		redisSecrets.Password = password.Value
	}
	if connectionString, ok := resource.SecretValues[renderers.ConnectionStringValue]; ok {
		redisSecrets.ConnectionString = connectionString.Value
	}
	if url, ok := resource.SecretValues[renderers.ConnectionURIValue]; ok {
		redisSecrets.URL = url.Value
	}

	versioned, err := converter.RedisCacheSecretsDataModelToVersioned(&redisSecrets, sCtx.APIVersion)
	if err != nil {
		return rest.NewBadRequestResponse(err.Error()), err
	}
	return rest.NewOKResponse(versioned), nil
}
