// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/rest"
	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
)

const (
	// This comes from the path: /Users/vinayada/radius/radius/swagger/specification/ucp/resource-manager/UCP/preview/2023-04-15-preview/openapi.json
	// This spec path is parsed and this string needs to be provider/resourceType.
	// For UCP, the provider is UCP and since all UCP resource types are in a single json, the file is named openapi.json.
	// Therefore, resourceType = ucp
	UCPEndpointType = "ucp/openapi"
	UCPApiVersion   = "2023-04-15-preview"
)

// APIValidator is the middleware to validate incoming request with OpenAPI spec.
func APIValidator(loader *Loader) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			rID, err := resources.ParseByMethod(r.URL.Path, r.Method)
			if err != nil {
				resp := invalidResourceIDResponse(r.URL.Path)
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}

			apiVersion := r.URL.Query().Get(APIVersionQueryKey)
			v, ok := loader.GetValidator(rID.Type(), apiVersion)
			if !ok {
				resp := unsupportedAPIVersionResponse(apiVersion, rID.Type(), loader.SupportedVersions(rID.Type()))
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}

			errs := v.ValidateRequest(r)
			if errs != nil {
				resp := validationFailedResponse(rID.Type()+"/"+rID.Name(), errs)
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

// APIValidatorUCP is the middleware to validate incoming request for UCP with OpenAPI spec.
func APIValidatorUCP(loader *Loader) func(h http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		fn := func(w http.ResponseWriter, r *http.Request) {
			endpointType := UCPEndpointType
			apiVersion := r.URL.Query().Get(APIVersionQueryKey)
			v, ok := loader.GetValidator(endpointType, apiVersion)
			if !ok {
				resp := unsupportedAPIVersionResponse(apiVersion, endpointType, loader.SupportedVersions(endpointType))
				if err := resp.Apply(r.Context(), w, r); err != nil {
					handleError(r.Context(), w, err)
				}
				return
			}

			errs := v.ValidateRequest(r)
			if errs != nil {
				resp := validationFailedResponse(endpointType, errs)
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
