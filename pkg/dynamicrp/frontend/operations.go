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
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// dynamicOperationHandler returns an http.Handler that can instantiate and run a controller.Controller for a dynamic resource.
//
// Usually when we register a route, we know up-front the resource type that will be handled by the controller.
// In the dynamic-rp use-case we don't know. We need to dynamically determine the resource type based on the URL.
//
// For example:
//
// Route: /planes/radius/{planeName}/providers/{providerNamespace}/locations/{locationName}/operationResults/{operationID}
// URL: /planes/radius/myplane/providers/Applications.Example/locations/global/operationResults/1234
// Resource Type: Applications.Example/operationResults
//
// # OR
//
// Route: /planes/radius/{planeName}/resourceGroups/my-rg/providers/{providerNamespace}/{resourceType}/{resourceName}
// URL: /planes/radius/myplane/resourceGroups/my-rg/providers/Applications.Example/customService/my-service
// Resource Type: Applications.Example/customService
//
// This code ensures that the controller will be provided with the correct resource type.
func dynamicOperationHandler(method v1.OperationMethod, baseOptions controller.Options, factory func(opts controller.Options) (controller.Controller, error)) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := resources.Parse(r.URL.Path)
		if err != nil {
			result := rest.NewBadRequestResponse(err.Error())
			err = result.Apply(r.Context(), w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			return
		}

		operationType := v1.OperationType{Type: strings.ToUpper(id.Type()), Method: method}

		// Copy the options and initalize them dynamically for this type.
		opts := baseOptions
		opts.ResourceType = id.Type()

		// Special case the operation status and operation result types.
		//
		// This is special-casing that all of our resource providers do to store a single data row for both operation statuses and operation results.
		if strings.HasSuffix(strings.ToLower(opts.ResourceType), "locations/operationstatuses") || strings.HasSuffix(strings.ToLower(opts.ResourceType), "locations/operationresults") {
			opts.ResourceType = id.ProviderNamespace() + "/operationstatuses"
		}

		ctrl, err := factory(opts)
		if err != nil {
			result := rest.NewBadRequestResponse(err.Error())
			err = result.Apply(r.Context(), w, r)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}

			return
		}

		handler := server.HandlerForController(ctrl, operationType)
		handler.ServeHTTP(w, r)
	})
}
