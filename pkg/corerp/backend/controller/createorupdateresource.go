// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
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
		return ctrl.Result{}, err
	}

	isNewResource := false
	if errors.Is(&store.ErrNotFound{}, err) {
		isNewResource = true
	}

	opType, _ := v1.ParseOperationType(request.OperationType)
	if opType.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}

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

	rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentDataModel, ok := dataModel.(rp.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(armerrors.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	oldOutputResources := deploymentDataModel.OutputResources()

	deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)

	if !isNewResource {
		diff := outputresource.GetGCOutputResources(deploymentDataModel.OutputResources(), oldOutputResources)
		err = c.DeploymentProcessor().Delete(ctx, id, diff)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	nr := &store.Object{
		Metadata: store.Metadata{
			ID: request.ResourceID,
		},
		Data: deploymentDataModel,
	}
	err = c.StorageClient().Save(ctx, nr, store.WithETag(obj.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
