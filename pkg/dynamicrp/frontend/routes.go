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

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
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

	r.Route(pathBase+"planes/radius/{planeName}/providers/{providerNamespace}", func(r chi.Router) {
		r.Route("/locations/{locationName}", func(r chi.Router) {
			r.Get("/{or:operation[Rr]esults}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationResultController))
			r.Get("/{os:operation[Ss]tatuses}/{operationID}", dynamicOperationHandler(v1.OperationGet, controllerOptions, makeGetOperationStatusController))
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
