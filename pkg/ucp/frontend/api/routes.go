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
	"net/http"

	"github.com/go-chi/chi/v5"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/ucp"
	kubernetes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/kubernetes"
	planes_ctrl "github.com/radius-project/radius/pkg/ucp/frontend/controller/planes"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/validator"
)

const (
	planeCollectionPath     = "/planes"
	planeTypeCollectionPath = "/planes/{planeType}"

	// OperationTypeKubernetesOpenAPIV2Doc is the operation type for the required OpenAPI v2 discovery document.
	//
	// This is required by the Kubernetes API Server.
	OperationTypeKubernetesOpenAPIV2Doc = "KUBERNETESOPENAPIV2DOC"

	// OperationTypeKubernetesOpenAPIV3Doc is the operation type for the required OpenAPI v3 discovery document.
	//
	// This is required by the Kubernetes API Server.
	OperationTypeKubernetesOpenAPIV3Doc = "KUBERNETESOPENAPIV3DOC"

	// OperationTypeKubernetesDiscoveryDoc is the operation type for the required Kubernetes API discovery document.
	OperationTypeKubernetesDiscoveryDoc = "KUBERNETESDISCOVERYDOC"

	// OperationTypePlanes is the operation type for the planes (all types) collection.
	OperationTypePlanes = "PLANES"
)

func initModules(ctx context.Context, mods []modules.Initializer) (map[string]http.Handler, []string, error) {
	logger := ucplog.FromContextOrDiscard(ctx)

	planeTypes := []string{}
	planeHandlers := map[string]http.Handler{}
	for _, module := range mods {
		logger.Info(fmt.Sprintf("Registering module for planeType %s", module.PlaneType()), "planeType", module.PlaneType())
		handler, err := module.Initialize(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to initialize module for plane type %s: %w", module.PlaneType(), err)
		}
		planeTypes = append(planeTypes, module.PlaneType())
		planeHandlers[module.PlaneType()] = handler
		logger.Info(fmt.Sprintf("Registered module for planeType %s", module.PlaneType()), "planeType", module.PlaneType())
	}

	return planeHandlers, planeTypes, nil
}

// Register registers the routes for UCP including modules.
func Register(ctx context.Context, router chi.Router, planeModules []modules.Initializer, options *ucp.Options) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info(fmt.Sprintf("Registering routes with path base: %s", options.Config.Server.PathBase))

	router.NotFound(validator.APINotFoundHandler())
	router.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	logger.Info("Initializing module handlers for planes.")
	moduleHandlers, registeredPlaneTypes, err := initModules(ctx, planeModules)
	if err != nil {
		return err
	}

	handlerOptions := []server.HandlerOptions{}
	// If we're in Kubernetes we have some required routes to implement.
	if options.Config.Server.PathBase != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.
		handlerOptions = append(handlerOptions, []server.HandlerOptions{
			{
				ParentRouter:      router,
				Path:              "/openapi/v2",
				OperationType:     &v1.OperationType{Type: OperationTypeKubernetesOpenAPIV2Doc, Method: v1.OperationGet},
				ResourceType:      OperationTypeKubernetesOpenAPIV2Doc,
				Method:            v1.OperationGet,
				ControllerFactory: kubernetes_ctrl.NewOpenAPIv2Doc,
			},
			{
				ParentRouter:      router,
				Path:              "/openapi/v3",
				OperationType:     &v1.OperationType{Type: OperationTypeKubernetesOpenAPIV3Doc, Method: v1.OperationGet},
				ResourceType:      OperationTypeKubernetesOpenAPIV3Doc,
				Method:            v1.OperationGet,
				ControllerFactory: kubernetes_ctrl.NewOpenAPIv3Doc,
			},
			{
				ParentRouter:      router,
				Path:              options.Config.Server.PathBase,
				OperationType:     &v1.OperationType{Type: OperationTypeKubernetesDiscoveryDoc, Method: v1.OperationGet},
				ResourceType:      OperationTypeKubernetesDiscoveryDoc,
				Method:            v1.OperationGet,
				ControllerFactory: kubernetes_ctrl.NewDiscoveryDoc,
			},
		}...)
	}

	// This router applies validation and will be used for CRUDL operations on planes
	apiValidator := validator.APIValidator(validator.Options{
		SpecLoader:         options.SpecLoader,
		ResourceTypeGetter: validator.UCPResourceTypeGetter,
	})

	// Configures planes collection and resource routes.
	planeCollectionRouter := server.NewSubrouter(router, options.Config.Server.PathBase+planeCollectionPath, apiValidator)

	// The "list all planes by type" handler is registered here.
	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		// Planes resource handler registration.
		{
			// This is a custom controller because we have to use custom query logic to list planes of all types.
			ParentRouter:      planeCollectionRouter,
			Method:            v1.OperationList,
			OperationType:     &v1.OperationType{Type: OperationTypePlanes, Method: v1.OperationList},
			ResourceType:      OperationTypePlanes,
			ControllerFactory: planes_ctrl.NewListPlanes,
		},
	}...)

	databaseClient, err := options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	ctrlOptions := controller.Options{
		Address:        options.Config.Server.Address(),
		DatabaseClient: databaseClient,
		PathBase:       options.Config.Server.PathBase,
		StatusManager:  options.StatusManager,

		KubeClient:   nil, // Unused by UCP
		ResourceType: "",  // Set dynamically
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOptions); err != nil {
			return err
		}
	}

	// Register a catch-all route to handle requests that get dispatched to a specific plane.
	unknownPlaneRouter := server.NewSubrouter(router, options.Config.Server.PathBase+planeTypeCollectionPath)
	unknownPlaneRouter.HandleFunc(server.CatchAllPath, func(w http.ResponseWriter, r *http.Request) {
		planeType := chi.URLParam(r, "planeType")
		handler, ok := moduleHandlers[planeType]
		if ok {
			logger := ucplog.FromContextOrDiscard(r.Context())
			logger.Info("Forwarding request to plane", "plane", planeType, "path", r.URL.Path, "method", r.Method)

			// Clear the route context in request context before forwarding the request to the module handler.
			chi.RouteContext(r.Context()).Reset()
			handler.ServeHTTP(w, r)
			return
		}

		// Handle invalid plane type error.
		resp := modules.InvalidPlaneTypeErrorResponse(planeType, registeredPlaneTypes)
		_ = resp.Apply(ctx, w, r)
	})

	return nil
}
