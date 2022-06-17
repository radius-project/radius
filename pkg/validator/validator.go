// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/go-openapi/errors"
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

func getRootScopePath(route *mux.Route) (string, error) {
	muxTemplate, err := route.GetPathTemplate()
	if err != nil {
		return "", err
	}
	return strings.Replace(muxTemplate, "{rootScope:.*}", "{rootScope}", 1), nil
}

func (v *validator) getParam(method, path string) map[string]spec.Parameter {
	path = strings.ToLower(path)
	v.paramsMu.RLock()
	p, ok := v.params[path]
	v.paramsMu.RUnlock()
	if ok {
		return p
	}

	v.paramsMu.Lock()
	defer v.paramsMu.Unlock()
	for k, _ := range v.specDoc.Analyzer.AllPaths() {
		if strings.Contains(strings.ToLower(k), path) {
			v.params[path] = v.specDoc.Analyzer.ParamsFor(method, k)
		}
	}

	return v.params[path]
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
	rootScopeRoute, err := getRootScopePath(mux.CurrentRoute(req))
	if err != nil {
		return routePathParseError(err)
	}

	routeParams := v.toRouteParams(req)

	params := v.getParam(req.Method, rootScopeRoute)
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

func invalidJSONError(err error) []ValidationError {
	return []ValidationError{{
		Message:   "invalid JSON error",
		JSONError: err,
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

func flattenComposite(errs *errors.CompositeError) *errors.CompositeError {
	var res []error
	for _, er := range errs.Errors {
		switch e := er.(type) {
		case *errors.CompositeError:
			if len(e.Errors) > 0 {
				flat := flattenComposite(e)
				if len(flat.Errors) > 0 {
					res = append(res, flat.Errors...)
				}
			}
		case *errors.Validation:
			if e != nil {
				res = append(res, e)
			}
		}
	}
	return errors.CompositeValidationError(res...)
}

func parseResult(result error) []ValidationError {
	errs := []ValidationError{}
	flattened := flattenComposite(result.(*errors.CompositeError))
	for _, e := range flattened.Errors {
		valErr, ok := e.(*errors.Validation)
		if ok {
			firstIndex := strings.Index(valErr.Name, ".")
			if firstIndex < 0 {
				firstIndex = 0
			}
			errs = append(errs, ValidationError{
				Position: valErr.Name[firstIndex:],
				Message:  valErr.Error(),
			})
		}
	}
	return errs
}
