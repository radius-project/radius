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
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/middleware"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	radius_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/radius"
	resourcegroups_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/resourcegroups"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	planeCollectionPath            = "/planes/radius"
	planeResourcePath              = "/planes/radius/{planeName}"
	resourceProviderCollectionPath = planeResourcePath + "/providers"
	resourceProviderResourcePath   = planeResourcePath + "/providers/{resourceProviderName}"
	resourceGroupCollectionPath    = planeResourcePath + "/resourcegroups"
	resourceGroupResourcePath      = planeResourcePath + "/resourcegroups/{resourceGroupName}"

	// OperationTypeUCPRadiusProxy is the operation type for proxying Radius API calls.
	OperationTypeUCPRadiusProxy = "UCPRADIUSPROXY"
)

func (m *Module) Initialize(ctx context.Context) (http.Handler, error) {
	ctrlOptions := controller.Options{
		Address:       m.options.Address,
		PathBase:      m.options.PathBase,
		DataProvider:  m.options.DataProvider,
		StatusManager: m.options.StatusManager,
	}

	baseRouter := server.NewSubrouter(m.router, m.options.PathBase)

	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         m.options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	planeResourceOptions := controller.ResourceOptions[datamodel.RadiusPlane]{
		RequestConverter:  converter.RadiusPlaneDataModelFromVersioned,
		ResponseConverter: converter.RadiusPlaneDataModelToVersioned,
	}

	// URLs for lifecycle of planes
	planeResourceType := "System.Radius/planes"
	planeCollectionRouter := server.NewSubrouter(baseRouter, planeCollectionPath, apiValidator)
	planeResourceRouter := server.NewSubrouter(baseRouter, planeResourcePath, apiValidator)

	resourceGroupResourceOptions := controller.ResourceOptions[datamodel.ResourceGroup]{
		RequestConverter:  converter.ResourceGroupDataModelFromVersioned,
		ResponseConverter: converter.ResourceGroupDataModelToVersioned,
	}

	// URLs for lifecycle of resource groups
	resourceGroupCollectionRouter := server.NewSubrouter(baseRouter, resourceGroupCollectionPath, apiValidator)
	resourceGroupResourceRouter := server.NewSubrouter(baseRouter, resourceGroupResourcePath, apiValidator)

	// URLs for lifecycle of resource providers
	resourceProviderCollectionRouter := server.NewSubrouter(baseRouter, resourceProviderCollectionPath, apiValidator)
	resourceProviderResourceRouter := server.NewSubrouter(baseRouter, resourceProviderResourcePath, apiValidator)

	handlerOptions := []server.HandlerOptions{
		{
			// This is a scope query so we can't use the default operation.
			ParentRouter:  planeCollectionRouter,
			Method:        v1.OperationList,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationList},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return &planes_ctrl.ListPlanesByType[*datamodel.RadiusPlane, datamodel.RadiusPlane]{
					Operation: controller.NewOperation(opts, planeResourceOptions),
				}, nil
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationGet,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationGet},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationPut,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationPut},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationDelete,
			OperationType: &v1.OperationType{Type: planeResourceType, Method: v1.OperationDelete},
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opts, planeResourceOptions)
			},
		},
		{
			ParentRouter: resourceProviderCollectionRouter,
			ResourceType: datamodel.ResourceProviderResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewListResources(opt,
					controller.ResourceOptions[datamodel.ResourceProvider]{
						RequestConverter:  converter.ResourceProviderDataModelFromVersioned,
						ResponseConverter: converter.ResourceProviderDataModelToVersioned,
					})
			},
			Middlewares: []func(http.Handler) http.Handler{middleware.OverrideResourceID(resourceIDForResourceProviderCollection(m.options.PathBase))},
		},
		{
			ParentRouter: resourceProviderResourceRouter,
			ResourceType: datamodel.ResourceProviderResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					controller.ResourceOptions[datamodel.ResourceProvider]{
						RequestConverter:  converter.ResourceProviderDataModelFromVersioned,
						ResponseConverter: converter.ResourceProviderDataModelToVersioned,
					},
				)
			},
			Middlewares: []func(http.Handler) http.Handler{middleware.OverrideResourceID(resourceIDForResourceProviderResource(m.options.PathBase))},
		},
		{
			ParentRouter: resourceProviderResourceRouter,
			ResourceType: datamodel.ResourceProviderResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					controller.ResourceOptions[datamodel.ResourceProvider]{
						RequestConverter:  converter.ResourceProviderDataModelFromVersioned,
						ResponseConverter: converter.ResourceProviderDataModelToVersioned,
					},
				)
			},
			Middlewares: []func(http.Handler) http.Handler{middleware.OverrideResourceID(resourceIDForResourceProviderResource(m.options.PathBase))},
		},
		{
			ParentRouter: resourceProviderResourceRouter,
			ResourceType: datamodel.ResourceProviderResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					controller.ResourceOptions[datamodel.ResourceProvider]{
						RequestConverter:  converter.ResourceProviderDataModelFromVersioned,
						ResponseConverter: converter.ResourceProviderDataModelToVersioned,
					},
				)
			},
			Middlewares: []func(http.Handler) http.Handler{middleware.OverrideResourceID(resourceIDForResourceProviderResource(m.options.PathBase))},
		},
		{
			ParentRouter:      resourceGroupCollectionRouter,
			ResourceType:      v20231001preview.ResourceGroupType,
			Method:            v1.OperationList,
			ControllerFactory: resourcegroups_ctrl.NewListResourceGroups,
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20231001preview.ResourceGroupType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opts, resourceGroupResourceOptions)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20231001preview.ResourceGroupType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opts, resourceGroupResourceOptions)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20231001preview.ResourceGroupType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opts controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opts, resourceGroupResourceOptions)
			},
		},
		{
			ParentRouter: resourceGroupResourceRouter,
			ResourceType: v20231001preview.GenericResourceType,
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
			ParentRouter:  resourceGroupResourceRouter,
			Path:          server.CatchAllPath,
			OperationType: &v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			ControllerFactory: func(o controller.Options) (controller.Controller, error) {
				return radius_ctrl.NewProxyController(o, m.Transport, m.EmbeddedTransport)
			},
		},
		{
			// Proxy request should use CatchAllPath(/*) to process all requests under /planes/radius/{planeName}/resourcegroups/{resourceGroupName}/providers/{resourceNamespace}/{resourceType}.
			ParentRouter:  resourceProviderResourceRouter,
			Path:          server.CatchAllPath,
			OperationType: &v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			ControllerFactory: func(o controller.Options) (controller.Controller, error) {
				return radius_ctrl.NewProxyController(o, m.Transport, m.EmbeddedTransport)
			},
		},
		{
			// Proxy request should use CatchAllPath(/*) to process all requests under /planes/radius/{planeName}/.
			ParentRouter:  planeResourceRouter,
			Path:          server.CatchAllPath,
			OperationType: &v1.OperationType{Type: OperationTypeUCPRadiusProxy, Method: v1.OperationProxy},
			ControllerFactory: func(o controller.Options) (controller.Controller, error) {
				return radius_ctrl.NewProxyController(o, m.Transport, m.EmbeddedTransport)
			},
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOptions); err != nil {
			return nil, err
		}
	}

	return m.router, nil
}

func resourceIDForResourceProviderCollection(pathBase string) func(req *http.Request) (resources.ID, error) {
	return func(req *http.Request) (resources.ID, error) {
		// URL should look like: /planes/radius/local/providers
		scope := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, pathBase), "/providers")
		return resources.Parse(scope + "/providers/System.Resources/resourceProviders")
	}
}

func resourceIDForResourceProviderResource(pathBase string) func(req *http.Request) (resources.ID, error) {
	return func(req *http.Request) (resources.ID, error) {
		// URL should look like: /planes/radius/local/providers/My.Namespaces
		scope, namespace, found := strings.Cut(strings.TrimPrefix(req.URL.Path, pathBase), "/providers/")
		if !found {
			return resources.ID{}, fmt.Errorf("unexpected resource provider URL: %s", req.URL.Path)
		}

		return resources.Parse(scope + "/providers/System.Resources/resourceProviders/" + namespace)
	}
}
