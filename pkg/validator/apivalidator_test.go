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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/swagger"
	"github.com/radius-project/radius/test/testutil"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

const (
	resourceGroupResource             = "/resourceGroups/{resourceGroupName}"
	environmentCollectionRoute        = "/providers/applications.core/environments"
	environmentResourceRoute          = "/providers/applications.core/environments/{environmentName}"
	armResourceGroupScopedResourceURL = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	ucpResourceGroupScopedResourceURL = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	longARMResourceURL                = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130"
	longUCPResourceURL                = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130"
	underscoreARMResourceURL          = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env_name0"
	underscoreUCPResourceURL          = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env_name0"
	digitARMResourceURL               = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/0env"
	digitUCPResourceURL               = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/0env"

	armResourceGroupScopedCollectionURL = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments"
	armSubscriptionScopedCollectionURL  = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/providers/applications.core/environments"
	ucpResourceGroupScopeCollectionURL  = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments"
	ucpPlaneScopedCollectionURL         = "http://localhost:8080/planes/radius/local/providers/applications.core/environments"

	operationGetRoute    = "/providers/applications.core/operations"
	subscriptionPUTRoute = "/subscriptions/{subscriptions}/resourceGroups/{resourceGroup}"
)

func Test_APIValidator_ARMID(t *testing.T) {
	runTest(t, armResourceGroupScopedResourceURL, "/subscriptions/", "/subscriptions/{subscriptionID}", []string{"/subscriptions/{subscriptionID}/resourceGroups/{resourceGroupName}", "/subscriptions/{subscriptionID}"})
}

func Test_APIValidator_UCPID(t *testing.T) {
	runTest(t, ucpResourceGroupScopedResourceURL, "/planes/", "/planes/{planeType}/{planeName}", []string{"/planes/{planeType}/{planeName}/resourceGroups/{resourceGroupName}", "/planes/{planeType}/{planeName}"})
}

func runTest(t *testing.T, resourceIDUrl, targetScope, planeRootScope string, prefixes []string) {
	// Load OpenAPI Spec for applications.core provider.
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, prefixes, "rootScope")

	require.NoError(t, err)

	validatorTests := []struct {
		desc       string
		method     string
		rootScope  string
		route      string
		apiVersion string

		contentFilePath string
		url             string
		responseCode    int
		validationErr   *v1.ErrorResponse
	}{
		{
			desc:         "not found route",
			method:       http.MethodGet,
			rootScope:    "",
			route:        operationGetRoute,
			apiVersion:   "2023-10-01-preview",
			url:          "http://localhost:8080/providers/applications.core/notfound",
			responseCode: http.StatusNotFound,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "NotFound",
					Message: "The request 'GET /providers/applications.core/notfound' is invalid.",
				},
			},
		},
		{
			desc:         "invalid http method",
			method:       http.MethodPut,
			rootScope:    "",
			route:        operationGetRoute,
			apiVersion:   "2023-10-01-preview",
			url:          "http://localhost:8080/providers/applications.core/operations",
			responseCode: http.StatusMethodNotAllowed,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "BadRequest",
					Message: "The request method 'PUT' is invalid.",
				},
			},
		},
		{
			desc:          "skip validation of /providers/applications.core/operations",
			method:        http.MethodGet,
			rootScope:     "",
			route:         operationGetRoute,
			apiVersion:    "2023-10-01-preview",
			url:           "http://localhost:8080/providers/applications.core/operations",
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:            "valid environment resource",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			desc:            "valid environment resource for selfhost",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid-selfhost.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			desc:          "valid get-environment",
			method:        http.MethodGet,
			rootScope:     planeRootScope + resourceGroupResource,
			route:         environmentResourceRoute,
			apiVersion:    "2023-10-01-preview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid list-environment with azure resource group",
			method:        http.MethodGet,
			rootScope:     planeRootScope + resourceGroupResource,
			route:         environmentCollectionRoute,
			apiVersion:    "2023-10-01-preview",
			url:           armResourceGroupScopedCollectionURL,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid list-environment with azure subscription",
			method:        http.MethodGet,
			rootScope:     planeRootScope,
			route:         environmentCollectionRoute,
			apiVersion:    "2023-10-01-preview",
			url:           armSubscriptionScopedCollectionURL,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid list-environment with UCP resource group",
			method:        http.MethodGet,
			rootScope:     planeRootScope + resourceGroupResource,
			route:         environmentCollectionRoute,
			apiVersion:    "2023-10-01-preview",
			url:           ucpResourceGroupScopeCollectionURL,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid list-environment with UCP plane",
			method:        http.MethodGet,
			rootScope:     planeRootScope,
			route:         environmentCollectionRoute,
			apiVersion:    "2023-10-01-preview",
			url:           ucpPlaneScopedCollectionURL,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid delete-environment with azure resource id",
			method:        http.MethodDelete,
			rootScope:     planeRootScope + resourceGroupResource,
			route:         environmentResourceRoute,
			apiVersion:    "2023-10-01-preview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:            "invalid put-environment with invalid api-version",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2022-06-20-privatepreview", // unsupported api version
			contentFilePath: "put-environments-valid.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "InvalidApiVersionParameter",
					Message: "API version '2022-06-20-privatepreview' for type 'applications.core/environments' is not supported. The supported api-versions are '2023-10-01-preview'.",
				},
			},
		},
		{
			desc:            "invalid put-environment with missing location property bag",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-invalid-missing-location.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidProperties",
							Message: "$.location in body is required",
						},
					},
				},
			},
		},
		{
			desc:            "invalid put-environment with missing kind property",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-invalid-missing-kind.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidProperties",
							Message: "$.properties.compute.kind in body is required",
						},
					},
				},
			},
		},
		{
			desc:            "invalid put-environment with multiple errors",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-invalid-missing-locationandkind.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidProperties",
							Message: "$.location in body is required",
						},
						{
							Code:    "InvalidProperties",
							Message: "$.properties.compute.kind in body is required",
						},
					},
				},
			},
		},
		{
			desc:            "invalid put-environment with invalid json doc",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-invalid-json.json",
			url:             resourceIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "The request content was invalid and could not be deserialized.",
						},
					},
				},
			},
		},
		{
			desc:            "env name too long",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             longARMResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should be at most 63 chars long",
						},
					},
				},
			},
		},
		{
			desc:            "env name too long",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             longUCPResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should be at most 63 chars long",
						},
					},
				},
			},
		},
		{
			desc:            "underscore not allowed in name",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             underscoreARMResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should match '^[A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9])?$'",
						},
					},
				},
			},
		},
		{
			desc:            "underscore not allowed in name",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             underscoreUCPResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should match '^[A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9])?$'",
						},
					},
				},
			},
		},
		{
			desc:            "name cannot start with digit",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             digitARMResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should match '^[A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9])?$'",
						},
					},
				},
			},
		},
		{
			desc:            "name cannot start with digit",
			method:          http.MethodPut,
			rootScope:       planeRootScope + resourceGroupResource,
			route:           environmentResourceRoute,
			apiVersion:      "2023-10-01-preview",
			contentFilePath: "put-environments-valid.json",
			url:             digitUCPResourceURL,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []v1.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "environmentName in path should match '^[A-Za-z]([-A-Za-z0-9]*[A-Za-z0-9])?$'",
						},
					},
				},
			},
		},
	}

	for _, tc := range validatorTests {
		t.Run(tc.desc, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := chi.NewRouter()

			r.NotFound(APINotFoundHandler())
			r.MethodNotAllowed(APIMethodNotAllowedHandler())

			// APIs undocumented in OpenAPI spec.

			r.MethodFunc(http.MethodGet, operationGetRoute, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})
			r.MethodFunc(http.MethodPut, "/subscriptions/{subscriptions}", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

			if tc.rootScope != "" {
				if !strings.Contains(tc.url, targetScope) {
					return
				}

				// Add API validator middleware
				validator := APIValidator(Options{
					SpecLoader:         l,
					ResourceTypeGetter: RadiusResourceTypeGetter,
				})

				subRouter := chi.NewRouter()
				// chi.Mount will create catch-all route (/*) for subRouter.
				r.Mount(tc.rootScope, subRouter)
				// Add API validator middleware to subRouter to validate IsCatchAll.
				subRouter.Use(validator)

				subRouter.Route(tc.route, func(r chi.Router) {
					r.Use(validator)
					r.MethodFunc(tc.method, "/", func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusAccepted)
					})
				})
			}

			// Load test fixture.
			var body []byte = []byte("")
			if tc.contentFilePath != "" {
				body = testutil.ReadFixture(tc.contentFilePath)
			}

			if tc.apiVersion != "" {
				tc.url += "?api-version=" + tc.apiVersion
			}

			req, err := http.NewRequestWithContext(context.Background(), tc.method, tc.url, bytes.NewBuffer(body))
			require.NoError(t, err)
			r.ServeHTTP(w, req)

			require.Equal(t, tc.responseCode, w.Result().StatusCode, "%s", w.Body.String())

			if w.Result().StatusCode != http.StatusAccepted {
				armErr := &v1.ErrorResponse{}
				err := json.Unmarshal(w.Body.Bytes(), armErr)
				require.NoError(t, err)
				require.Equal(t, tc.validationErr, armErr)
			}
		})
	}
}
