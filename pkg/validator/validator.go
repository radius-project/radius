// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"errors"
	"fmt"
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

// ValidationError represents a validation error.
type ValidationError struct {
	// Position contains the field position, e.g. (root),
	// (root).location, (root).properties.resources.0
	//
	// It could be unset, in case the object was not valid JSON.
	Position string

	// Message contains the error message, e.g. "location is required".
	Message string

	// JSONError contains the parsing error if the provided document
	// wasn't valid JSON.
	JSONError error
}

// Validator validates HTTP request.
type Validator interface {
	// ValidateRequest validates a http request and returns all the errors.
	ValidateRequest(req *http.Request) []ValidationError
}

type validator struct {
	TypeName   string
	APIVersion string

	specDoc  *loads.Document
	params   map[string]map[string]spec.Parameter
	paramsMu *sync.RWMutex
}

func (v *validator) getParam(req *http.Request) (map[string]spec.Parameter, error) {
	route := mux.CurrentRoute(req)
	if route == nil {
		return nil, errors.New("route is nil")
	}
	pathTemplate, err := route.GetPathTemplate()
	if err != nil {
		return nil, err
	}

	v.paramsMu.RLock()
	p, ok := v.params[pathTemplate]
	v.paramsMu.RUnlock()
	if ok {
		return p, nil
	}

	v.paramsMu.Lock()
	defer v.paramsMu.Unlock()

	// Gorilla mux route path should start with {rootScope;.*} to handle UCP and Azure root scope.
	scopePath := strings.Replace(pathTemplate, "{rootScope:.*}", "{rootScope}", 1)
	var param map[string]spec.Parameter = nil
	for k := range v.specDoc.Analyzer.AllPaths() {
		if strings.EqualFold(k, scopePath) {
			param = v.specDoc.Analyzer.ParamsFor(req.Method, k)
		}
	}
	if param != nil {
		v.params[pathTemplate] = param
		return v.params[pathTemplate], nil
	}
	return nil, ErrUndefinedAPI
}

func (v *validator) toRouteParams(req *http.Request) middleware.RouteParams {
	params := mux.Vars(req)

	routeParams := middleware.RouteParams{}
	for k, v := range params {
		routeParams = append(routeParams, middleware.RouteParam{Name: k, Value: v})
	}

	return routeParams
}

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
		Message: "failed to parse route: " + err.Error(),
	}}
}

type AggregateValidationError struct {
	Details []ValidationError
}

func (v *AggregateValidationError) Error() string {
	var message strings.Builder
	fmt.Fprintln(&message, "failed validation(s):")
	for _, err := range v.Details {
		if err.JSONError != nil {
			// The given document isn't even JSON.
			fmt.Fprintf(&message, "- %s: %v\n", err.Message, err.JSONError)
		} else {
			fmt.Fprintf(&message, "- %s: %s\n", err.Position, err.Message)
		}
	}
	return message.String()
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

func parseResult(result error) []ValidationError {
	errs := []ValidationError{}
	flattened := flattenComposite(result.(*oai_errors.CompositeError))
	for _, e := range flattened.Errors {
		valErr, ok := e.(*oai_errors.Validation)
		if ok {
			firstIndex := 0
			if valErr.In == "body" {
				firstIndex = strings.Index(valErr.Name, ".")
				if firstIndex < 0 {
					firstIndex = 0
				}
			}
			errs = append(errs, ValidationError{
				Position: valErr.Name[firstIndex:],
				Message:  valErr.Error(),
			})
		}
	}
	return errs
}
