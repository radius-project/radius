// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"errors"
	"net/http"
	"strings"
	"sync"

	oai_errors "github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
	"github.com/gorilla/mux"
)

const (
	InvalidRequestContentCode   = "InvalidRequestContent"
	InvalidObjectPropertiesCode = "InvalidObjectProperties"
)

// ValidationError represents a validation error.
type ValidationError struct {
	// Code represents the code of validation error.
	Code string

	// Message contains the error message, e.g. "location is required".
	Message string
}

// Validator validates HTTP request.
type Validator interface {
	// ValidateRequest validates a http request and returns all the errors.
	ValidateRequest(req *http.Request) []ValidationError
}

type validator struct {
	TypeName   string
	APIVersion string

	specDoc      *loads.Document
	paramCache   map[string]map[string]spec.Parameter
	paramCacheMu *sync.RWMutex
}

// getParam looks up the correct spec.Parameter which a unique parameter is defined by a combination
// of a [name](#parameterName) and [location](#parameterIn). This spec.Parameter are loaded from swagger
// file and consumed by middleware.NewUntypedRequestBinder. To fetch spec.Parameter, we need to get
// the case-sensitive route path which is defined in swagger file. getParam first gets route defined
// by gorilla mux, replace {rootScope:.*} in gorilla mux route with {rootScope} and iterate the loaded
// parameters from swagger file to find the matched route path defined in swagger file. Then it caches
// spec.Parameter for the next lookup.
func (v *validator) getParam(req *http.Request) (map[string]spec.Parameter, error) {
	route := mux.CurrentRoute(req)
	if route == nil {
		return nil, errors.New("route is nil")
	}
	// Fetch gorilla mux route path from the current request.
	pathTemplate, err := route.GetPathTemplate()
	if err != nil {
		return nil, err
	}

	v.paramCacheMu.RLock()
	p, ok := v.paramCache[pathTemplate]
	v.paramCacheMu.RUnlock()
	if ok {
		return p, nil
	}

	v.paramCacheMu.Lock()
	defer v.paramCacheMu.Unlock()

	// Gorilla mux route path should start with {rootScope:.*} to handle UCP and Azure root scope.
	scopePath := strings.Replace(pathTemplate, "{rootScope:.*}", "{rootScope}", 1)
	var param map[string]spec.Parameter = nil
	// Iterate loaded paths to find the matched route.
	for k := range v.specDoc.Analyzer.AllPaths() {
		if strings.EqualFold(k, scopePath) {
			param = v.specDoc.Analyzer.ParamsFor(req.Method, k)
		}
	}
	if param != nil {
		v.paramCache[pathTemplate] = param
		return v.paramCache[pathTemplate], nil
	}
	return nil, ErrUndefinedAPI
}

// toRouteParams converts gorilla mux params to go-openapi RouteParams to validate parameters.
func (v *validator) toRouteParams(req *http.Request) middleware.RouteParams {
	params := mux.Vars(req)

	routeParams := middleware.RouteParams{}
	for k, v := range params {
		routeParams = append(routeParams, middleware.RouteParam{Name: k, Value: v})
	}

	return routeParams
}

// ValidateRequest validates http.Request and returns ValidationError if the request is invalid.
func (v *validator) ValidateRequest(req *http.Request) []ValidationError {
	routeParams := v.toRouteParams(req)
	params, err := v.getParam(req)
	if err != nil {
		return routePathParseError(err)
	}

	binder := middleware.NewUntypedRequestBinder(params, v.specDoc.Spec(), strfmt.Default)
	data := map[string]interface{}{}
	var errs []ValidationError
	result := binder.Bind(req, middleware.RouteParams(routeParams), runtime.JSONConsumer(), &data)
	if result != nil {
		errs = parseResult(result)
	}

	return errs
}

func routePathParseError(err error) []ValidationError {
	return []ValidationError{{
		Code:    InvalidRequestContentCode,
		Message: "failed to parse route: " + err.Error(),
	}}
}

func parseResult(result error) []ValidationError {
	errs := []ValidationError{}
	flattened := flattenComposite(result.(*oai_errors.CompositeError))
	for _, e := range flattened.Errors {
		valErr, ok := e.(*oai_errors.Validation)
		if ok {
			ve := ValidationError{
				Message: valErr.Error(),
			}

			if valErr.In == "body" {
				period := strings.Index(valErr.Name, ".")
				if period < 0 {
					ve.Code = InvalidRequestContentCode
					ve.Message = "The request content was invalid and could not be deserialized."
				} else {
					name := valErr.Name[:period]
					ve.Code = InvalidObjectPropertiesCode
					ve.Message = strings.ReplaceAll(ve.Message, name, "$")
				}
			}

			errs = append(errs, ve)
		}
	}
	return errs
}

func flattenComposite(errs *oai_errors.CompositeError) *oai_errors.CompositeError {
	var res []error
	for _, er := range errs.Errors {
		switch e := er.(type) {
		case *oai_errors.CompositeError:
			if len(e.Errors) > 0 {
				flat := flattenComposite(e)
				if len(flat.Errors) > 0 {
					res = append(res, flat.Errors...)
				}
			}
		case *oai_errors.Validation:
			if e != nil {
				res = append(res, e)
			}
		}
	}
	return oai_errors.CompositeValidationError(res...)
}
