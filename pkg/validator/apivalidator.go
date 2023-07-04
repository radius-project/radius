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

package validator

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	// This comes from the path: /Users/vinayada/radius/radius/swagger/specification/ucp/resource-manager/UCP/preview/2022-09-01-privatepreview/openapi.json
	// This spec path is parsed and this string needs to be provider/resourceType.
	// For UCP, the provider is UCP and since all UCP resource types are in a single json, the file is named openapi.json.
	// Therefore, resourceType = ucp
	UCPEndpointType = "ucp/openapi"
	UCPApiVersion   = "2022-09-01-privatepreview"
)

// Options represents the options for APIValidator.
type Options struct {
	// SpecLoader is the loader to load the OpenAPI spec.
	SpecLoader *Loader

	// ResourceType is the function to get the resource type from the request.
	ResourceTypeGetter func(*http.Request) (string, error)
}

// RadiusResourceTypeGetter is the function to get the resource type for Radius resource ID.
func RadiusResourceTypeGetter(r *http.Request) (string, error) {
	resourceID, err := resources.ParseByMethod(r.URL.Path, r.Method)
	if err != nil {
		return "", err
	}
	return resourceID.Type(), nil
}

// UCPEndpointTypeGetter is the function to get the resource type for UCP.
func UCPResourceTypeGetter(r *http.Request) (string, error) {
	return UCPEndpointType, nil
}

// APIValidator is the middleware to validate incoming request with OpenAPI spec.
func APIValidator(options Options) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			// Skip validation for catch-all requests.
			if isCatchAllRoute(r) {
				h.ServeHTTP(w, r)
				return
			}

			if options.ResourceTypeGetter == nil {
				panic("options.ResourceType must be specified")
			}

			resourceType, err := options.ResourceTypeGetter(r)
			if err != nil {
				resp := invalidResourceIDResponse(r.URL.Path)
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}

			apiVersion := r.URL.Query().Get(APIVersionQueryKey)
			v, ok := options.SpecLoader.GetValidator(resourceType, apiVersion)
			if !ok {
				resp := unsupportedAPIVersionResponse(apiVersion, resourceType, options.SpecLoader.SupportedVersions(resourceType))
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}

			errs := v.ValidateRequest(r)
			if errs != nil {
				resp := validationFailedResponse(resourceType, errs)
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}
			h.ServeHTTP(w, r)
		}

		return http.HandlerFunc(fn)
	}
}

// isCatchAllRoute returns true if the request is a catch-all request. If the matched patterns are layered with the multiple routers,
// the matched pattern which doesn't include "/*" suffix is the last pattern.
func isCatchAllRoute(r *http.Request) bool {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return false
	}

	patternLen := len(rctx.RoutePatterns)
	if patternLen > 0 {
		lastPattern := rctx.RoutePatterns[patternLen-1]
		if strings.HasSuffix(lastPattern, "/*") {
			return true
		}
	}

	return false
}

func invalidResourceIDResponse(id string) rest.Response {
	return rest.NewBadRequestARMResponse(v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    v1.CodeInvalid,
			Message: fmt.Sprintf("Invalid Resource ID: %s", id),
		},
	})
}

func unsupportedAPIVersionResponse(apiVersion, resourceType string, supportedAPIVersions []string) rest.Response {
	return rest.NewBadRequestARMResponse(v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    v1.CodeInvalidApiVersionParameter,
			Message: fmt.Sprintf("API version '%s' for type '%s' is not supported. The supported api-versions are '%s'.", apiVersion, resourceType, strings.Join(supportedAPIVersions, ", ")),
		},
	})
}

func validationFailedResponse(qualifiedName string, valErrs []ValidationError) rest.Response {
	errDetails := []v1.ErrorDetails{}

	for _, verr := range valErrs {
		errDetails = append(errDetails, v1.ErrorDetails{Code: verr.Code, Message: verr.Message})
	}

	resp := rest.NewBadRequestARMResponse(v1.ErrorResponse{
		Error: v1.ErrorDetails{
			Code:    v1.CodeHTTPRequestPayloadAPISpecValidationFailed,
			Target:  qualifiedName,
			Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
			Details: errDetails,
		},
	})

	return resp
}

func handleError(ctx context.Context, w http.ResponseWriter, err error) {
	logger := ucplog.FromContextOrDiscard(ctx)
	w.WriteHeader(http.StatusInternalServerError)
	logger.Error(err, "error writing marshaled data to output")
}

// APINotFoundHandler is the handler when the request url route does not exist
//
//	r := mux.NewRouter()
//	r.NotFoundHandler = APINotFoundHandler()
func APINotFoundHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		restResponse := rest.NewNotFoundMessageResponse(fmt.Sprintf("The request '%s %s' is invalid.", r.Method, r.URL.Path))
		if err := restResponse.Apply(r.Context(), w, r); err != nil {
			handleError(r.Context(), w, err)
		}
	}
}

// APIMethodNotAllowedHandler is the handler when the request method does not match the route.
//
//	r := mux.NewRouter()
//	r.MethodNotAllowedHandler = APIMethodNotAllowedHandler()
func APIMethodNotAllowedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		target := ""
		if rID, err := resources.Parse(r.URL.Path); err != nil {
			target = rID.Type() + "/" + rID.Name()
		}
		restResponse := rest.NewMethodNotAllowedResponse(target, fmt.Sprintf("The request method '%s' is invalid.", r.Method))
		if err := restResponse.Apply(r.Context(), w, r); err != nil {
			handleError(r.Context(), w, err)
		}
	}
}
