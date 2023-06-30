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
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"
	oai_errors "github.com/go-openapi/errors"
	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	APIVersionQueryKey = "api-version"
)

var (
	ErrUndefinedRoute = errors.New("undefined route path")
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

	rootScopePrefixes []string
	rootScopeParam    string
	specDoc           *loads.Document
	paramCache        map[string]map[string]spec.Parameter
	paramCacheMu      *sync.RWMutex
}

// findParam looks up the correct spec.Parameter which a unique parameter is defined by a combination
// of a [name](#parameterName) and [location](#parameterIn). This spec.Parameter are loaded from swagger
// file and consumed by middleware.NewUntypedRequestBinder. To fetch spec.Parameter, we need to get
// the case-sensitive route path which is defined in swagger file. findParam first gets route defined
// by gorilla mux, replace {rootScope:.*} in gorilla mux route with {rootScope} and iterate the loaded
// parameters from swagger file to find the matched route path defined in swagger file. Then it caches
// spec.Parameter for the next lookup to improve the performance.
func (v *validator) findParam(req *http.Request) (map[string]spec.Parameter, error) {
	// Fetch route path from the current request.
	rctx := chi.RouteContext(req.Context())
	if rctx == nil {
		return nil, errors.New("chi.RouteContext is nil")
	}

	pathTemplate := rctx.RoutePattern()

	templateKey := req.Method + "-" + pathTemplate
	v.paramCacheMu.RLock()
	p, ok := v.paramCache[templateKey]
	v.paramCacheMu.RUnlock()
	if ok {
		return p, nil
	}

	v.paramCacheMu.Lock()
	defer v.paramCacheMu.Unlock()
	// Return immediately if the previous call fills the cache.
	p, ok = v.paramCache[templateKey]
	if ok {
		return p, nil
	}

	// The Gorilla mux route path for our RPs should start with {rootScope:.*} to handle UCP and Azure root scope.
	//
	// The UCP functionality like resource groups does not have a "/{rootScope}/" in the path.
	// Need to handle this difference in the CoreRP vs UCP schema.
	var scopePath string
	replaceToken := "/{" + v.rootScopeParam + "}"
	if v.rootScopeParam == "" {
		replaceToken = v.rootScopeParam
	}

	for _, prefix := range v.rootScopePrefixes {
		if strings.HasPrefix(pathTemplate, prefix) {
			scopePath = strings.Replace(pathTemplate, prefix, replaceToken, 1)
			break
		}
	}

	// Iterate loaded paths to find the matched route.
	for k := range v.specDoc.Analyzer.AllPaths() {
		if strings.EqualFold(k, scopePath) {
			v.paramCache[templateKey] = v.specDoc.Analyzer.ParamsFor(req.Method, k)
			return v.paramCache[templateKey], nil
		}
	}
	return nil, ErrUndefinedRoute
}

// toRouteParams converts gorilla mux params to go-openapi RouteParams to validate parameters.
func (v *validator) toRouteParams(req *http.Request) middleware.RouteParams {
	routeParams := middleware.RouteParams{}

	if rID, err := resources.Parse(req.URL.Path); err == nil {
		routeParams = append(routeParams, middleware.RouteParam{Name: v.rootScopeParam, Value: rID.RootScope()})
	}
	for k := range req.URL.Query() {
		routeParams = append(routeParams, middleware.RouteParam{Name: k, Value: req.URL.Query().Get(k)})
	}

	rctx := chi.RouteContext(req.Context())
	if rctx == nil {
		return routeParams
	}

	for i := 0; i < len(rctx.URLParams.Keys); i++ {
		routeParams = append(routeParams, middleware.RouteParam{Name: rctx.URLParams.Keys[i], Value: rctx.URLParams.Values[i]})
	}

	return routeParams
}

// ValidateRequest validates http.Request and returns []ValidationError if the request is invalid.
// Known limitation:
//   - readonly property: go-openapi/middleware doesn't support "readonly" property even though
//     go-openapi/validate has readonly property check used only for go-swagger.
//     (xeipuuv/gojsonschema and kin-openapi doesn't support readonly either)
func (v *validator) ValidateRequest(req *http.Request) []ValidationError {
	routeParams := v.toRouteParams(req)
	params, err := v.findParam(req)
	if err != nil {
		if errors.Is(err, ErrUndefinedRoute) {
			return []ValidationError{{
				Code:    v1.CodeInvalidRequestContent,
				Message: "failed to parse route: " + err.Error(),
			}}
		}
	}

	binder := middleware.NewUntypedRequestBinder(params, v.specDoc.Spec(), strfmt.Default)
	var errs []ValidationError

	// Read content for validation and recover later.
	defer req.Body.Close()
	content, err := io.ReadAll(req.Body)
	if err != nil {
		return []ValidationError{{
			Code:    v1.CodeInvalidRequestContent,
			Message: "failed to read body content: " + err.Error(),
		}}
	}

	// WORKAROUND: https://github.com/project-radius/radius/issues/2683
	// UCP or DE sends the invalid request which has -1 ContentLength header so validator treats it as empty content.
	if req.ContentLength < 0 && len(content) > 0 {
		req.ContentLength = (int64)(len(content))
	}

	bindData := make(map[string]any)
	result := binder.Bind(
		req, middleware.RouteParams(routeParams),
		// Pass content to the validator marshaler to prevent from reading body from buffer.
		runtime.ConsumerFunc(func(reader io.Reader, data any) error {
			return json.Unmarshal(content, data)
		}), bindData)
	if result != nil {
		errs = parseResult(result)
	}

	// Recover body after validation is done.
	req.Body = io.NopCloser(bytes.NewBuffer(content))

	return errs
}

func parseResult(result error) []ValidationError {
	errs := []ValidationError{}
	flattened := flattenComposite(result.(*oai_errors.CompositeError))
	for _, e := range flattened.Errors {
		valErr, ok := e.(*oai_errors.Validation)
		if ok {
			ve := ValidationError{
				Code:    v1.CodeInvalidRequestContent,
				Message: valErr.Error(),
			}

			if valErr.In == "body" {
				period := strings.Index(valErr.Name, ".")
				if period < 0 {
					// invalid json body.
					if valErr.Code() == oai_errors.InvalidTypeCode {
						ve.Message = "The request content was invalid and could not be deserialized."
					}
				} else {
					// go-openapi returns the error position "EnvironmentResource.properties.compute.kind" starting with
					// definition name of the body schema. This replaces the definition name with $ to avoid the confusion.
					// For example, "EnvironmentResource.properties.compute.kind" -> "$.properties.compute.kind"
					name := valErr.Name[:period]
					ve.Code = v1.CodeInvalidProperties
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
