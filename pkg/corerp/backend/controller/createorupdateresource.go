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
	"errors"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/corerp/renderers/container"
	"github.com/project-radius/radius/pkg/corerp/renderers/gateway"
	"github.com/project-radius/radius/pkg/corerp/renderers/httproute"
	"github.com/project-radius/radius/pkg/corerp/renderers/volume"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*CreateOrUpdateResource)(nil)

// CreateOrUpdateResource is the async operation controller to create or update Applications.Core/Containers resource.
type CreateOrUpdateResource struct {
	ctrl.BaseController
}

// NewCreateOrUpdateResource creates a new CreateOrUpdateResource controller.
func NewCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &CreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func getDataModel(id resources.ID) (v1.DataModelInterface, error) {
	resourceType := strings.ToLower(id.Type())
	switch resourceType {
	case strings.ToLower(container.ResourceType):
		return &datamodel.ContainerResource{}, nil
	case strings.ToLower(gateway.ResourceType):
		return &datamodel.Gateway{}, nil
	case strings.ToLower(httproute.ResourceType):
		return &datamodel.HTTPRoute{}, nil
	case strings.ToLower(volume.ResourceType):
		return &datamodel.VolumeResource{}, nil
	default:
		return nil, fmt.Errorf("invalid resource type: %q for dependent resource ID: %q", resourceType, id.String())
	}
}

// Run checks if the resource exists, renders the resource, deploys the resource, applies the
// deployment output to the resource, deletes any resources that are no longer needed, and saves the resource.
func (c *CreateOrUpdateResource) Run(ctx context.Context, request *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, request.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{ID: request.ResourceID}, err) {
		return ctrl.Result{}, err
	}

	isNewResource := false
	if errors.Is(&store.ErrNotFound{ID: request.ResourceID}, err) {
		isNewResource = true
	}

	opType, _ := v1.ParseOperationType(request.OperationType)
	if opType.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{ID: request.ResourceID}, err) {
		return ctrl.Result{}, err
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

	rendererOutput, err := c.DeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentOutput, err := c.DeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentDataModel, ok := dataModel.(rpv1.DeploymentDataModel)
	if !ok {
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	oldOutputResources := deploymentDataModel.OutputResources()

	err = deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !isNewResource {
		diff := rpv1.GetGCOutputResources(deploymentDataModel.OutputResources(), oldOutputResources)
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
