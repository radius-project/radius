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
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/swagger"
	"github.com/project-radius/radius/test/testutil"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/require"
)

const (
	envRoute           = "/providers/applications.core/environments/{environmentName}"
	armIDUrl           = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	ucpIDUrl           = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	longarmIDUrl       = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130"
	longucpIDUrl       = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130"
	underscorearmIDUrl = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env_name0"
	underscoreucpIDUrl = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env_name0"
	digitarmIDUrl      = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/0env"
	digitucpIDUrl      = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/0env"

	operationGetRoute    = "/providers/applications.core/operations"
	subscriptionPUTRoute = "/subscriptions/{subscriptions}/resourceGroups/{resourceGroup}"
)

func TestAPIValidator_ARMID(t *testing.T) {
	runTest(t, armIDUrl)
}

func TestAPIValidator_UCPID(t *testing.T) {
	runTest(t, ucpIDUrl)
}

func runTest(t *testing.T, resourceIDUrl string) {
	// Load OpenAPI Spec for applications.core provider.
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, "/{rootScope:.*}", "rootScope")

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
			apiVersion:   "2022-03-15-privatepreview",
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
			apiVersion:   "2022-03-15-privatepreview",
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
			apiVersion:    "2022-03-15-privatepreview",
			url:           "http://localhost:8080/providers/applications.core/operations",
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:            "skip validation of /subscriptions/{subscriptionID}",
			method:          http.MethodPut,
			rootScope:       "",
			route:           subscriptionPUTRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000",
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			desc:            "valid environment resource",
			method:          http.MethodPut,
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			desc:            "valid environment resource for selfhost",
			method:          http.MethodPut,
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid-selfhost.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			desc:          "valid get-environment with azure resource id",
			method:        http.MethodGet,
			rootScope:     "/{rootScope:.*}",
			route:         envRoute,
			apiVersion:    "2022-03-15-privatepreview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid delete-environment with azure resource id",
			method:        http.MethodDelete,
			rootScope:     "/{rootScope:.*}",
			route:         envRoute,
			apiVersion:    "2022-03-15-privatepreview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:            "invalid put-environment with invalid api-version",
			method:          http.MethodPut,
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-06-20-privatepreview", // unsupported api version
			contentFilePath: "put-environments-valid.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "InvalidApiVersionParameter",
					Message: "API version '2022-06-20-privatepreview' for type 'applications.core/environments' is not supported. The supported api-versions are '2022-03-15-privatepreview'.",
				},
			},
		},
		{
			desc:            "invalid put-environment with missing location property bag",
			method:          http.MethodPut,
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-missing-location.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-missing-kind.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-missing-locationandkind.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-json.json",
			url:             resourceIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             longarmIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             longucpIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/largeEnvName14161820222426283032343638404244464850525456586062646668707274767880828486889092949698100102104106108120122124126128130",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             underscorearmIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env_name0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             underscoreucpIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env_name0",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             digitarmIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/0env",
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
			rootScope:       "/{rootScope:.*}",
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             digitucpIDUrl,
			responseCode:    400,
			validationErr: &v1.ErrorResponse{
				Error: v1.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/0env",
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
			r := mux.NewRouter()

			r.NotFoundHandler = APINotFoundHandler()
			r.MethodNotAllowedHandler = APIMethodNotAllowedHandler()

			// APIs undocumented in OpenAPI spec.
			r.Path(operationGetRoute).Methods(http.MethodGet).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})
			r.Path("/subscriptions/{subscriptions}").Methods(http.MethodPut).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

			if tc.rootScope != "" {
				// Add API validator middleware
				validator := APIValidator(l)
				router := r.PathPrefix(tc.rootScope).Subrouter()
				// Register validator at {rootScope} level
				router.Use(validator)

				router.Path(tc.route).Methods(tc.method).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
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

			req, _ := http.NewRequestWithContext(context.Background(), tc.method, tc.url, bytes.NewBuffer(body))
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
