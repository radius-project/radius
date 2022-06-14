package main

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
)

var envInputBody = `
{
	"location": "West US",
      "properties": {
        "compute": {
          "kinid": "kubernetes",
          "resourceId": "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/testGroup/providers/Microsoft.ContainerService/managedClusters/radiusTestCluster"
        }
      }
	}
`

type validationError struct {
	Name    string
	In      string
	Value   interface{}
	Message string
}

func main() {
	// Load OpenAPI Spec
	specDoc, err := loads.JSONSpec("../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json")
	if err != nil {
		panic(err)
	}
	// Expand external references.
	wDoc, err := specDoc.Expanded(&spec.ExpandOptions{
		RelativeBase: "../../swagger/specification/applications/resource-manager/Applications.Core/preview/2022-03-15-privatepreview/environments.json",
	})

	// Fake HTTP request
	params := wDoc.Analyzer.ParamsFor("PUT", "/{rootScope}/providers/Applications.Core/environments/{environmentName}")
	binder := middleware.NewUntypedRequestBinder(params, specDoc.Spec(), strfmt.Default)
	req, _ := http.NewRequest("PUT", "http://localhost:8002/subscriptions/subid/resourceGroups/rg/providers/Applications.Core/environments/env0", bytes.NewBufferString(envInputBody))
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

func parseResult(result error) []validationError {
	errs := []validationError{}
	flattened := flattenComposite(result.(*errors.CompositeError))
	for _, e := range flattened.Errors {
		valErr, ok := e.(*errors.Validation)
		if ok {
			errs = append(errs, validationError{
				Name:    valErr.Name,
				In:      valErr.In,
				Value:   valErr.Value,
				Message: valErr.Error(),
			})
		}
	}
	return errs
}
