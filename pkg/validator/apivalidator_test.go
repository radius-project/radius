// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package validator

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	radiustesting "github.com/project-radius/radius/pkg/corerp/testing"
	"github.com/project-radius/radius/pkg/radrp/armerrors"
	"github.com/project-radius/radius/swagger"
	"github.com/stretchr/testify/require"
)

const (
	envRoute             = "/providers/applications.core/environments/{environmentName}"
	armIDUrl             = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	ucpIDUrl             = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
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
	l, err := LoadSpec(context.Background(), "applications.core", swagger.SpecFiles, "/{rootScope:.*}")

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
		validationErr   *armerrors.ErrorResponse
	}{
		{
			desc:         "not found route",
			method:       http.MethodGet,
			rootScope:    "",
			route:        operationGetRoute,
			apiVersion:   "2022-03-15-privatepreview",
			url:          "http://localhost:8080/providers/applications.core/notfound",
			responseCode: http.StatusNotFound,
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []armerrors.ErrorDetails{
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
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []armerrors.ErrorDetails{
						{
							Code:    "InvalidRequestContent",
							Message: "The request content was invalid and could not be deserialized.",
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
				body = radiustesting.ReadFixture(tc.contentFilePath)
			}

			if tc.apiVersion != "" {
				tc.url += "?api-version=" + tc.apiVersion
			}

			req, _ := http.NewRequestWithContext(context.Background(), tc.method, tc.url, bytes.NewBuffer(body))
			r.ServeHTTP(w, req)

			require.Equal(t, tc.responseCode, w.Result().StatusCode, "%s", w.Body.String())

			if w.Result().StatusCode != http.StatusAccepted {
				armErr := &armerrors.ErrorResponse{}
				err := json.Unmarshal(w.Body.Bytes(), armErr)
				require.NoError(t, err)
				require.Equal(t, tc.validationErr, armErr)
			}
		})
	}
}
