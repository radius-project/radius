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
	"github.com/project-radius/radius/pkg/connectorrp/datamodel"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteResource)(nil)

// DeleteResource is the async operation controller to delete Applications.Connector resource.
type DeleteResource struct {
	ctrl.BaseController
}

// NewDeleteResource creates the DeleteResource controller instance.
func NewDeleteResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &DeleteResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *DeleteResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: err.Error()}), err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	resourceData, err := getResourceData(id, obj, serviceCtx)
	if err != nil {
		return ctrl.Result{}, err
	}
	err = c.ConnectorRPDeploymentProcessor().Delete(ctx, resourceData)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = c.StorageClient().Delete(ctx, request.ResourceID)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}

func getResourceData(id resources.ID, obj *store.Object, serviceCtx *v1.ARMRequestContext) (deployment.ResourceData, error) {
	resourceType := strings.ToLower(id.Type())
	switch resourceType {
	case strings.ToLower(mongodatabases.ResourceTypeName):
		d := &datamodel.MongoDatabase{}
		if err := obj.As(d); err != nil {
			return deployment.ResourceData{}, err
		}
		return deployment.ResourceData{ID: serviceCtx.ResourceID, Resource: d, OutputResources: d.Properties.Status.OutputResources, ComputedValues: d.ComputedValues, SecretValues: d.SecretValues, RecipeData: d.RecipeData}, nil
	default:
		return deployment.ResourceData{}, fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, id.String())
	}
}
