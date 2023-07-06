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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	dapr_dm "github.com/project-radius/radius/pkg/daprrp/datamodel"
	ds_dm "github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/processors"
	msg_dm "github.com/project-radius/radius/pkg/messagingrp/datamodel"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the async operation controller to delete Applications.Link resource.
type DeleteResource struct {
	ctrl.BaseController
	client processors.ResourceClient
}

// NewDeleteResource creates the DeleteResource controller instance.
func NewDeleteResource(opts ctrl.Options, client processors.ResourceClient) (ctrl.Controller, error) {
	return &DeleteResource{ctrl.NewBaseAsyncController(opts), client}, nil
}

// # Function Explanation
//
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

	deploymentDataModel, ok := dataModel.(rpv1.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), nil
	}

	err = c.deleteResources(ctx, id.String(), deploymentDataModel.OutputResources())
	if err != nil {
		return ctrl.Result{}, err
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
	case strings.ToLower(linkrp.MongoDatabasesResourceType):
		return &datamodel.MongoDatabase{}, nil
	case strings.ToLower(linkrp.RedisCachesResourceType):
		return &datamodel.RedisCache{}, nil
	case strings.ToLower(linkrp.SqlDatabasesResourceType):
		return &datamodel.SqlDatabase{}, nil
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		return &datamodel.DaprStateStore{}, nil
	case strings.ToLower(linkrp.RabbitMQMessageQueuesResourceType):
		return &datamodel.RabbitMQMessageQueue{}, nil
	case strings.ToLower(linkrp.DaprSecretStoresResourceType):
		return &datamodel.DaprSecretStore{}, nil
	case strings.ToLower(linkrp.DaprPubSubBrokersResourceType):
		return &datamodel.DaprPubSubBroker{}, nil
	case strings.ToLower(linkrp.ExtendersResourceType):
		return &datamodel.Extender{}, nil
	case strings.ToLower(linkrp.N_MongoDatabasesResourceType):
		return &ds_dm.MongoDatabase{}, nil
	case strings.ToLower(linkrp.N_RedisCachesResourceType):
		return &ds_dm.RedisCache{}, nil
	case strings.ToLower(linkrp.N_SqlDatabasesResourceType):
		return &ds_dm.SqlDatabase{}, nil
	case strings.ToLower(linkrp.N_DaprStateStoresResourceType):
		return &dapr_dm.DaprStateStore{}, nil
	case strings.ToLower(linkrp.N_RabbitMQQueuesResourceType):
		return &msg_dm.RabbitMQQueue{}, nil
	case strings.ToLower(linkrp.N_DaprSecretStoresResourceType):
		return &dapr_dm.DaprSecretStore{}, nil
	case strings.ToLower(linkrp.N_DaprPubSubBrokersResourceType):
		return &dapr_dm.DaprPubSubBroker{}, nil
	default:
		return nil, fmt.Errorf("async delete operation unsupported on resource type: %q. Resource ID: %q", resourceType, id.String())
	}
}

func (d *DeleteResource) deleteResources(ctx context.Context, id string, outputResources []rpv1.OutputResource) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	orderedOutputResources, err := rpv1.OrderOutputResources(outputResources)
	if err != nil {
		return err
	}

	// Loop over each output resource and delete in reverse dependency order
	for i := len(orderedOutputResources) - 1; i >= 0; i-- {
		outputResource := orderedOutputResources[i]
		id := outputResource.Identity.GetID()
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Deleting output resource: %v, LocalID: %s, resource type: %s\n", outputResource.Identity, outputResource.LocalID, outputResource.ResourceType.Type))
		if outputResource.RadiusManaged == nil || !*outputResource.RadiusManaged {
			continue
		}

		err = d.client.Delete(ctx, id, resourcemodel.APIVersionUnknown)
		if err != nil {
			return err
		}
		logger.Info(fmt.Sprintf("Deleted output resource: %q", id), ucplog.LogFieldTargetResourceID, id)

	}

	return nil
}
