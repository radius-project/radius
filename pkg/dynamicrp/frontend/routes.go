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

package frontend

import (
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/validator"
)

func (s *Service) registerRoutes(r *chi.Mux, controllerOptions controller.Options) error {
	// Return ARM errors for invalid requests.
	r.NotFound(validator.APINotFoundHandler())
	r.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	// Return ARM errors for invalid requests.
	r.NotFound(validator.APINotFoundHandler())
	r.MethodNotAllowed(validator.APIMethodNotAllowedHandler())

	pathBase := s.options.Config.Server.PathBase
	if pathBase == "" {
		pathBase = "/"
	}

	if !strings.HasSuffix(pathBase, "/") {
		pathBase = pathBase + "/"
	}

	r.Route(pathBase+"planes/radius/{planeName}", func(r chi.Router) {

		// Plane-scoped
		r.Route("/providers/{providerNamespace}", func(r chi.Router) {

			// Plane-scoped LIST operation
			r.Get("/{resourceType}", dynamicOperationHandler(v1.OperationPlaneScopeList, controllerOptions, makeListResourceAtPlaneScopeController))

			// Async operation status/results
			r.Route("/locations/{locationName}", func(r chi.Router) {
				r.Get("/{or:operation[Rr]esults}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationResultController))
				r.Get("/{os:operation[Ss]tatuses}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationStatusController))
			})
		})

		// Resource-group-scoped
		r.Route("/{rg:resource[gG]roups}/{resourceGroupName}/providers/{providerNamespace}/{resourceType}", func(r chi.Router) {
			r.Get("/", dynamicOperationHandler(v1.OperationList, controllerOptions, makeListResourceAtResourceGroupScopeController))
			r.Get("/{resourceName}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetResourceController))
			r.Put("/{resourceName}", dynamicOperationHandler(v1.OperationPut, controllerOptions, makePutResourceController))
			r.Delete("/{resourceName}", dynamicOperationHandler(v1.OperationDelete, controllerOptions, makeDeleteResourceController))
		})
	})

	return nil
}

var dynamicResourceOptions = controller.ResourceOptions[datamodel.DynamicResource]{
	RequestConverter:         converter.DynamicResourceDataModelFromVersioned,
	ResponseConverter:        converter.DynamicResourceDataModelToVersioned,
	AsyncOperationRetryAfter: time.Second * 5,
	AsyncOperationTimeout:    time.Hour * 24,
}

func makeListResourceAtPlaneScopeController(opts controller.Options) (controller.Controller, error) {
	// At plane scope we list resources recursively to include all resource groups.
	copy := dynamicResourceOptions
	copy.ListRecursiveQuery = true
	return defaultoperation.NewListResources(opts, copy)
}

func makeListResourceAtResourceGroupScopeController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewListResources(opts, dynamicResourceOptions)
}

func makeGetResourceController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewGetResource(opts, dynamicResourceOptions)
}

func makePutResourceController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewDefaultAsyncPut(opts, dynamicResourceOptions)
}

func makeDeleteResourceController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewDefaultAsyncDelete(opts, dynamicResourceOptions)
}

func makeGetOperationResultController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewGetOperationResult(opts)
}

func makeGetOperationStatusController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewGetOperationStatus(opts)
}
