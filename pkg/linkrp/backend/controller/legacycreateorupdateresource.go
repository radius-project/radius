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
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/asyncoperation/controller"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*LegacyCreateOrUpdateResource)(nil)

// LegacyCreateOrUpdateResource is the async operation controller to create or update Applications.Link resources.
type LegacyCreateOrUpdateResource struct {
	ctrl.BaseController
}

// NewLegacyCreateOrUpdateResource creates the CreateOrUpdateResource controller instance.
func NewLegacyCreateOrUpdateResource(opts ctrl.Options) (ctrl.Controller, error) {
	return &LegacyCreateOrUpdateResource{ctrl.NewBaseAsyncController(opts)}, nil
}

func (c *LegacyCreateOrUpdateResource) Run(ctx context.Context, req *ctrl.Request) (ctrl.Result, error) {
	obj, err := c.StorageClient().Get(ctx, req.ResourceID)
	if err != nil && !errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}

	isNewResource := false
	if errors.Is(&store.ErrNotFound{}, err) {
		isNewResource = true
	}

	opType, _ := v1.ParseOperationType(req.OperationType)
	if opType.Method == http.MethodPatch && errors.Is(&store.ErrNotFound{}, err) {
		return ctrl.Result{}, err
	}

	// This code is general and we might be processing an async job for a resource or a scope, so using the general Parse function.
	id, err := resources.Parse(req.ResourceID)
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
		return ctrl.NewFailedResult(v1.ErrorDetails{Message: "deployment data model conversion error"}), err
	}

	rendererOutput, err := c.LinkDeploymentProcessor().Render(ctx, id, dataModel)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploymentOutput, err := c.LinkDeploymentProcessor().Deploy(ctx, id, rendererOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	oldOutputResources := deploymentDataModel.OutputResources()
	err = deploymentDataModel.ApplyDeploymentOutput(deploymentOutput)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !isNewResource {
		diff := rpv1.GetGCOutputResources(deploymentDataModel.OutputResources(), oldOutputResources)
		err = c.LinkDeploymentProcessor().Delete(ctx, id, diff)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	nr := &store.Object{
		Metadata: store.Metadata{
			ID: req.ResourceID,
		},
		Data: deploymentDataModel,
	}
	err = c.StorageClient().Save(ctx, nr, store.WithETag(obj.ETag))
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, err
}
