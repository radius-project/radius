// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// CreateOrUpdateResource is the async operation controller to create or update Applications.Core/Containers resource.
type CreateOrUpdateResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
func NewCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func getDataModel(id resources.ID) (conv.DataModelInterface, error) {
	resourceType := strings.ToLower(id.Type())
	switch resourceType {
	case strings.ToLower(container.ResourceType):
		return &datamodel.ContainerResource{}, nil
	case strings.ToLower(gateway.ResourceType):
		return &datamodel.Gateway{}, nil
	case strings.ToLower(httproute.ResourceType):
		return &datamodel.HTTPRoute{}, nil
	default:
		return nil, fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, id.String())
	}
}

func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	dataModel, err := getDataModel(id)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	if err = obj.As(dataModel); err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	deploymentDataModel, ok := dataModel.(rp.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)

	_, err = c.SaveResource(ctx, request.ResourceID, deploymentDataModel, obj.ETag)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}
	return ctrl.Result{}, err
}
