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

package api

import (
	"context"
	"fmt"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	kubernetes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/kubernetes"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
)

const (
	planeCollectionPath       = "/planes"
	planeCollectionByTypePath = "/planes/{planeType}"
	planeResourcePath         = "/planes/{planeType}/{planeName}"
	planePrefixPathFmt        = "/planes/%s/{planeName}"

	// OperationTypeKubernetesOpenAPIV2Doc is the operation type for the required OpenAPI v2 discovery document.
	//
	// This is required by the Kubernetes API Server.
	OperationTypeKubernetesOpenAPIV2Doc = "KUBERNETESOPENAPIV2DOC"

	// OperationTypeKubernetesDiscoveryDoc is the operation type for the required Kubernetes API discovery document.
	OperationTypeKubernetesDiscoveryDoc = "KUBERNETESDISCOVERYDOC"

	// OperationTypePlanes is the operation type for the planes (all types) collection.
	OperationTypePlanes = "PLANES"

	// OperationTypePlanes is the operation type for the planes (specific type) endpoints
	OperationTypePlanesByType = "PLANESBYTYPE"
)

// Register registers the routes for UCP including modules.
//
// # Function Explanation
//
// This function registers handlers for various operations on Azure and AWS, such as Get, Put, Delete, and List,
// as well as a catch-all route for proxying.
func Register(ctx context.Context, router *mux.Router, modules []modules.Initializer, options modules.Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Registering routes with path base: %s", options.PathBase))

	router.NotFoundHandler = validator.APINotFoundHandler()
	router.MethodNotAllowedHandler = validator.APIMethodNotAllowedHandler()

	handlerOptions := []server.HandlerOptions{}

	// If we're in Kubernetes we have some required routes to implement.
	if options.PathBase != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.
		handlerOptions = append(handlerOptions, []server.HandlerOptions{
			{
				ParentRouter:      router.Path("/openapi/v2").Subrouter(),
				OperationType:     &v1.OperationType{Type: OperationTypeKubernetesOpenAPIV2Doc, Method: v1.OperationGet},
				Method:            v1.OperationGet,
				ControllerFactory: kubernetes_ctrl.NewOpenAPIv2Doc,
			},
			{
				ParentRouter:      router.Path(options.PathBase).Subrouter(),
				OperationType:     &v1.OperationType{Type: OperationTypeKubernetesDiscoveryDoc, Method: v1.OperationGet},
				Method:            v1.OperationGet,
				ControllerFactory: kubernetes_ctrl.NewDiscoveryDoc,
			},
		}...)
	}

	// This router applies validation and will be used for CRUDL operations on planes
	rootScopeRouter := router.PathPrefix(options.PathBase).Name("subrouter: <pathbase>").Subrouter()
	rootScopeRouter.Use(validator.APIValidatorUCP(options.SpecLoader))

	planeCollectionRouter := rootScopeRouter.Path(planeCollectionPath).Subrouter()
	planeCollectionByTypeRouter := rootScopeRouter.Path(planeCollectionByTypePath).Subrouter()
	planeResourceRouter := rootScopeRouter.Path(planeResourcePath).Subrouter()

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		// Planes resource handler registration.
		{
			// This is scope query unlike the default list handler.
			ParentRouter:      planeCollectionRouter,
			Method:            v1.OperationList,
			OperationType:     &v1.OperationType{Type: OperationTypePlanes, Method: v1.OperationList},
			ControllerFactory: planes_ctrl.NewListPlanes,
		},
		{
			// This is scope query unlike the default list handler.
			ParentRouter:      planeCollectionByTypeRouter,
			Method:            v1.OperationList,
			OperationType:     &v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationList},
			ControllerFactory: planes_ctrl.NewListPlanesByType,
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationGet,
			OperationType: &v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationGet},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					controller.ResourceOptions[datamodel.Plane]{
						RequestConverter:  converter.PlaneDataModelFromVersioned,
						ResponseConverter: converter.PlaneDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationPut,
			OperationType: &v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationPut},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncPut(opt,
					controller.ResourceOptions[datamodel.Plane]{
						RequestConverter:  converter.PlaneDataModelFromVersioned,
						ResponseConverter: converter.PlaneDataModelToVersioned,
					},
				)
			},
		},
		{
			ParentRouter:  planeResourceRouter,
			Method:        v1.OperationDelete,
			OperationType: &v1.OperationType{Type: OperationTypePlanesByType, Method: v1.OperationDelete},
			ControllerFactory: func(opt controller.Options) (controller.Controller, error) {
				return defaultoperation.NewDefaultSyncDelete(opt,
					controller.ResourceOptions[datamodel.Plane]{
						RequestConverter:  converter.PlaneDataModelFromVersioned,
						ResponseConverter: converter.PlaneDataModelToVersioned,
					},
				)
			},
		},
	}...)

	ctrlOptions := controller.Options{
		Address:      options.Address,
		PathBase:     options.PathBase,
		DataProvider: options.DataProvider,
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOptions); err != nil {
			return err
		}
	}

	for _, module := range modules {
		logger.Info(fmt.Sprintf("Registering module for planeType %s", module.PlaneType()), "planeType", module.PlaneType())
		handler, err := module.Initialize(ctx)
		if err != nil {
			return fmt.Errorf("failed to initialize module for plane type %s: %w", module.PlaneType(), err)
		}

		name := fmt.Sprintf("subrouter: <pathbase>/planes/%s", module.PlaneType())
		router.PathPrefix(options.PathBase + fmt.Sprintf(planePrefixPathFmt, module.PlaneType())).Name(name).Handler(handler)
		logger.Info(fmt.Sprintf("Registered module for planeType %s", module.PlaneType()), "planeType", module.PlaneType())
	}

	return nil
}
