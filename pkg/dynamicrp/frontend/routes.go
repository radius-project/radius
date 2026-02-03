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
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel/converter"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/validator"
)

func (s *Service) registerRoutes(
	r *chi.Mux,
	controllerOptions controller.Options,
	ucpClient *v20231001preview.ClientFactory,
	handler *encryption.SensitiveDataHandler,
) error {
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

	// Create encryption filter for sensitive fields
	encryptionFilter := makeEncryptionFilter(ucpClient, handler)

	// Resource options with encryption filter applied to PUT operations
	resourceOptions := controller.ResourceOptions[datamodel.DynamicResource]{
		RequestConverter:  converter.DynamicResourceDataModelFromVersioned,
		ResponseConverter: converter.DynamicResourceDataModelToVersioned,
		UpdateFilters: []controller.UpdateFilter[datamodel.DynamicResource]{
			encryptionFilter,
		},
		AsyncOperationRetryAfter: time.Second * 5,
		AsyncOperationTimeout:    time.Hour * 24,
	}

	r.Route(pathBase+"planes/radius/{planeName}", func(r chi.Router) {

		// Plane-scoped
		r.Route("/providers/{providerNamespace}", func(r chi.Router) {

			// Plane-scoped LIST operation
			r.Get("/{resourceType}", dynamicOperationHandler(v1.OperationPlaneScopeList, controllerOptions,
				func(opts controller.Options) (controller.Controller, error) {
					optsCopy := resourceOptions
					optsCopy.ListRecursiveQuery = true
					return defaultoperation.NewListResources(opts, optsCopy)
				}))

			// Async operation status/results
			r.Route("/locations/{locationName}", func(r chi.Router) {
				r.Get("/{or:operation[Rr]esults}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationResultController))
				r.Get("/{os:operation[Ss]tatuses}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationStatusController))
			})
		})

		// Resource-group-scoped
		r.Route("/{rg:resource[gG]roups}/{resourceGroupName}/providers/{providerNamespace}/{resourceType}", func(r chi.Router) {
			r.Get("/", dynamicOperationHandler(v1.OperationList, controllerOptions,
				func(opts controller.Options) (controller.Controller, error) {
					return defaultoperation.NewListResources(opts, resourceOptions)
				}))
			r.Get("/{resourceName}", dynamicOperationHandler(v1.OperationGet, controllerOptions,
				func(opts controller.Options) (controller.Controller, error) {
					return defaultoperation.NewGetResource(opts, resourceOptions)
				}))
			r.Put("/{resourceName}", dynamicOperationHandler(v1.OperationPut, controllerOptions,
				func(opts controller.Options) (controller.Controller, error) {
					return defaultoperation.NewDefaultAsyncPut(opts, resourceOptions)
				}))
			r.Delete("/{resourceName}", dynamicOperationHandler(v1.OperationDelete, controllerOptions,
				func(opts controller.Options) (controller.Controller, error) {
					return defaultoperation.NewDefaultAsyncDelete(opts, resourceOptions)
				}))
		})
	})

	return nil
}

func makeGetOperationResultController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewGetOperationResult(opts)
}

func makeGetOperationStatusController(opts controller.Options) (controller.Controller, error) {
	return defaultoperation.NewGetOperationStatus(opts)
}
