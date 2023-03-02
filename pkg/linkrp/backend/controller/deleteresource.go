// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the async operation controller to delete Applications.Link resource.
type DeleteResource struct {
	ctrl.BaseController
}

// NewDeleteResource creates the DeleteResource controller instance.
func NewDeleteResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *DeleteResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("DeleteResource.Run()")
	fmt.Printf("DeleteResource.Run() - resourceID: %s\n", request.ResourceID)

	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("DeleteResource - id: ", "id", id.String())

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

	err = c.LinkDeploymentProcessor().Delete(ctx, id, deploymentDataModel.OutputResources())
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
	case strings.ToLower(linkrp.DaprStateStoresResourceType):
		return &datamodel.DaprStateStore{}, nil
	default:
		return nil, fmt.Errorf("async delete operation unsupported on resource type: %q. Resource ID: %q", resourceType, id.String())
	}
}
