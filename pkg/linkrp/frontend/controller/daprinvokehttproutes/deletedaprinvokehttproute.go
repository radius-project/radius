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

package daprinvokehttproutes

import (
	"context"
	"errors"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/ucp/store"
)

var _ ctrl.Controller = (*DeleteDaprInvokeHttpRoute)(nil)

// DeleteDaprInvokeHttpRoute is the controller implementation to delete daprInvokeHttpRoute link resource.
type DeleteDaprInvokeHttpRoute struct {
	ctrl.Operation[*datamodel.DaprInvokeHttpRoute, datamodel.DaprInvokeHttpRoute]
	dp deployment.DeploymentProcessor
}

// NewDeleteDaprInvokeHttpRoute creates a new instance DeleteDaprInvokeHttpRoute.
func NewDeleteDaprInvokeHttpRoute(opts frontend_ctrl.Options) (ctrl.Controller, error) {
	return &DeleteDaprInvokeHttpRoute{
		Operation: ctrl.NewOperation(opts.Options,
			ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
				RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
				ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
			}),
		dp: opts.DeployProcessor,
	}, nil
}

func (daprHttpRoute *DeleteDaprInvokeHttpRoute) Run(ctx context.Context, w http.ResponseWriter, req *http.Request) (rest.Response, error) {
	serviceCtx := v1.ARMRequestContextFromContext(ctx)

	old, etag, err := daprHttpRoute.GetResource(ctx, serviceCtx.ResourceID)
	if err != nil {
		return nil, err
	}

	if old == nil {
		return rest.NewNoContentResponse(), nil
	}

	if etag == "" {
		return rest.NewNoContentResponse(), nil
	}

	r, err := daprHttpRoute.PrepareResource(ctx, req, nil, old, etag)
	if r != nil || err != nil {
		return r, err
	}

	err = daprHttpRoute.dp.Delete(ctx, serviceCtx.ResourceID, old.Properties.Status.OutputResources)
	if err != nil {
		return nil, err
	}

	err = daprHttpRoute.StorageClient().Delete(ctx, serviceCtx.ResourceID.String())
	if err != nil {
		if errors.Is(&store.ErrNotFound{}, err) {
			return rest.NewNoContentResponse(), nil
		}
		return nil, err
	}

	return rest.NewOKResponse(nil), nil
}
