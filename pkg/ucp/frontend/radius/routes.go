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

package radius

import (
	"context"
	"net/http"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp/api/v20220901privatepreview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	resourcegroups_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	planeScope                  = "/planes/{planeType}/{planeName}"
	resourceGroupCollectionPath = "/resourcegroups"
	resourceGroupResourcePath   = "/resourcegroups/{resourceGroupName}"

	// OperationTypeUCPRadiusProxy is the operation type for proxying Radius API calls.
	OperationTypeUCPRadiusProxy = "UCPRADIUSPROXY"
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	baseRouter := server.NewSubrouter(m.router, m.options.PathBase+planeScope)

	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         m.options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	// URLs for lifecycle of resource groups
	resourceGroupCollectionRouter := server.NewSubrouter(baseRouter, resourceGroupCollectionPath, apiValidator)
	resourceGroupResourceRouter := server.NewSubrouter(baseRouter, resourceGroupResourcePath, apiValidator)

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter:      resourceGroupCollectionRouter,
			ResourceType:      v20220901privatepreview.ResourceGroupType,
			Method:            v1.OperationList,
			ControllerFactory: resourcegroups_ctrl.NewListResourceGroups,
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20220901privatepreview.ResourceGroupType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					controller.ResourceOptions[datamodel.ResourceGroup]{
						RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
						ResponseConverter: converter.ResourceGroupDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20220901privatepreview.ResourceGroupType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					controller.ResourceOptions[datamodel.ResourceGroup]{
						RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
						ResponseConverter: converter.ResourceGroupDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20220901privatepreview.ResourceGroupType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					controller.ResourceOptions[datamodel.ResourceGroup]{
						RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
						ResponseConverter: converter.ResourceGroupDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20220901privatepreview.ResourceType,
			Path:         "/resources",
			Method:       v1.OperationList,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return resourcegroups_ctrl.NewListResources(opt)
			},
		},
		// Chi router uses radix tree so that it doesn't linear search the matched one. So, to catch all requests,
		// we need to use CatchAllPath(/*) at the above matched routes path in chi router.
		//
		// Note that the API validation is not applied for CatchAllPath(/*).
		{
			// Proxy request should use CatchAllPath(/*) to process all requests under /planes/radius/{planeName}/resourcegroups/{resourceGroupName}.
			ParentRouter:      resourceGroupResourceRouter,
			Path:              server.CatchAllPath,
			OperationType:     &v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			ControllerFactory: planes_ctrl.NewProxyController,
		},
		{
			// Proxy request should use CatchAllPath(/*) to process all requests under /planes/radius/{planeName}/.
			ParentRouter:      baseRouter,
			Path:              server.CatchAllPath,
			OperationType:     &v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			ControllerFactory: planes_ctrl.NewProxyController,
		},
	}

	ctrlOptions := controller.Options{
		Address:      m.options.Address,
		PathBase:     m.options.PathBase,
		DataProvider: m.options.DataProvider,
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOptions); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}
