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
	"fmt"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	ext_dm "github.com/radius-project/radius/pkg/corerp/datamodel"
	dapr_dm "github.com/radius-project/radius/pkg/daprrp/datamodel"
	ds_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	"github.com/radius-project/radius/pkg/portableresources"
	"github.com/radius-project/radius/pkg/portableresources/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/engine"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the async operation controller to delete a portable resource.
type DeleteResource struct {
	ctrl.BaseController
	engine engine.Engine
}

// NewDeleteResource creates a new DeleteResource controller which is used to delete resources asynchronously.
func NewDeleteResource(opts ctrl.Options, engine engine.Engine) (ctrl.Controller, error) {
	return &DeleteResource{ctrl.NewBaseAsyncController(opts), engine}, nil
}

// Run retrieves a resource from storage, parses the resource ID, gets the data model, deletes the output
// resources, and deletes the resource from storage. It returns an error if any of these steps fail.
func (c *DeleteResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	dataModel, err := getDataModel(id)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err = obj.As(dataModel); err != nil {
		return ctrl.Result{}, err
	}

	resourceDataModel, ok := dataModel.(rpv1.RadiusResourceModel)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), nil
	}

	recipeDataModel, supportsRecipes := dataModel.(datamodel.RecipeDataModel)
	if supportsRecipes && recipeDataModel.Recipe() != nil {
		recipeData := recipes.ResourceMetadata{
			Name:          recipeDataModel.Recipe().Name,
			EnvironmentID: resourceDataModel.ResourceMetadata().Environment,
			ApplicationID: resourceDataModel.ResourceMetadata().Application,
			Parameters:    recipeDataModel.Recipe().Parameters,
			ResourceID:    id.String(),
		}

		err = c.engine.Delete(ctx, recipeData, resourceDataModel.OutputResources())
		if err != nil {
			if recipeError, ok := err.(*recipes.RecipeError); ok {
				return ctrl.NewFailedResult(recipeError.ErrorDetails), nil
			}
			return ctrl.Result{}, err
		}
	}

	err = c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func getDataModel(id resources.ID) (v1.ResourceDataModel, error) {
	resourceType := strings.ToLower(id.Type())
	switch resourceType {
	case strings.ToLower(portableresources.MongoDatabasesResourceType):
		return &ds_dm.MongoDatabase{}, nil
	case strings.ToLower(portableresources.RedisCachesResourceType):
		return &ds_dm.RedisCache{}, nil
	case strings.ToLower(portableresources.SqlDatabasesResourceType):
		return &ds_dm.SqlDatabase{}, nil
	case strings.ToLower(portableresources.DaprStateStoresResourceType):
		return &dapr_dm.DaprStateStore{}, nil
	case strings.ToLower(portableresources.RabbitMQQueuesResourceType):
		return &msg_dm.RabbitMQQueue{}, nil
	case strings.ToLower(portableresources.DaprSecretStoresResourceType):
		return &dapr_dm.DaprSecretStore{}, nil
	case strings.ToLower(portableresources.DaprPubSubBrokersResourceType):
		return &dapr_dm.DaprPubSubBroker{}, nil
	case strings.ToLower(portableresources.ExtendersResourceType):
		return &ext_dm.Extender{}, nil
	default:
		return nil, fmt.Errorf("async delete operation unsupported on resource type: %q. Resource ID: %q", resourceType, id.String())
	}
}
