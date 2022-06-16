// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package swagger

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/strfmt"
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
	TypeName string
	specDoc  loads.Document
}

func (v *validator) ValidateRequest(req *http.Request) []ValidationError {
	// Fake HTTP request
	params := v.specDoc.Analyzer.ParamsFor("PUT", "/{rootScope}/providers/Applications.Core/environments/{environmentName}")
	binder := middleware.NewUntypedRequestBinder(params, v.specDoc.Spec(), strfmt.Default)
	data := map[string]interface{}{}

	// Need to populate the parameters using gorilla mux
	routeParams := []middleware.RouteParam{
		{"environmentName", "env0"},
		{"rootScope", "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup"},
		{"api-version", "2022-03-15-privatepreview"},
	}

	// Validate!
	result := binder.Bind(req, middleware.RouteParams(routeParams), runtime.JSONConsumer(), &data)
	if result != nil {
		// Flatten the validation errors.
		errs := parseResult(result)
		fmt.Printf("\n\n%v", errs)
		// Output example:
		// [{api-version query  api-version in query is required} {EnvironmentResource.properties.compute.kind body <nil> EnvironmentResource.properties.compute.kind in body is required}]
	}

	return nil
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
			errs = append(errs, ValidationError{
				Position: valErr.Name,
				Message:  valErr.Error(),
			})
		}
	}
	return errs
}
