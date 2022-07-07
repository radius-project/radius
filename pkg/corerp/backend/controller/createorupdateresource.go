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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
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

// Tried with Generics => No Luck Yet!
// type AsyncResource interface {
// 	*datamodel.ContainerResource | *datamodel.HTTPRoute | *datamodel.Gateway
// }

// type DataModel[T AsyncResource] struct {
// 	data T
// }

func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	id, err := resources.Parse(request.ResourceID)
	if err != nil {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	resourceType := id.Type()
	switch resourceType {
	case strings.ToLower(container.ResourceType):
		cr := &datamodel.ContainerResource{}
		if err = obj.As(cr); err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, cr)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		cr.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.DeployedOutputResources
		cr.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
		cr.InternalMetadata.SecretValues = deploymentOutput.SecretValues

		_, err = c.SaveResource(ctx, request.ResourceID, cr, obj.ETag)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}
	case strings.ToLower(gateway.ResourceType):
		gw := &datamodel.Gateway{}
		if err = obj.As(gw); err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, gw)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		gw.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.DeployedOutputResources
		gw.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
		gw.InternalMetadata.SecretValues = deploymentOutput.SecretValues

		_, err = c.SaveResource(ctx, request.ResourceID, gw, obj.ETag)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}
	case strings.ToLower(httproute.ResourceType):
		hr := &datamodel.HTTPRoute{}
		if err = obj.As(hr); err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, hr)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}

		hr.Properties.BasicResourceProperties.Status.OutputResources = deploymentOutput.DeployedOutputResources
		hr.InternalMetadata.ComputedValues = deploymentOutput.ComputedValues
		hr.InternalMetadata.SecretValues = deploymentOutput.SecretValues

		_, err = c.SaveResource(ctx, request.ResourceID, hr, obj.ETag)
		if err != nil {
			return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
		}
	default:
		err = fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, id.String())
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: err.Error()}), err
	}

	res := ctrl.Result{}
	res.SetProvisioningState(v1.ProvisioningStateSucceeded)
	return res, err
}
