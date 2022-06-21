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
	envRoute  = "/{rootScope:.*}/providers/applications.core/environments/{environmentName}"
	armIDUrl  = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	ucpIDUrl  = "http://localhost:8080/planes/radius/local/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
	skipRoute = "/providers/applications.core/operations"
)

func TestAPIValidator_ARMID(t *testing.T) {
	runTest(t, armIDUrl)
}

func TestAPIValidator_UCPID(t *testing.T) {
	runTest(t, ucpIDUrl)
}

func runTest(t *testing.T, resourceIDUrl string) {
	// Load OpenAPI Spec for applications.core provider.
	l := NewLoader("applications.core", swagger.SpecFiles)
	err := l.LoadSpec()

	require.NoError(t, err)

	validatorTests := []struct {
		desc       string
		method     string
		route      string
		apiVersion string

		contentFilePath string
		url             string
		responseCode    int
		validationErr   *armerrors.ErrorResponse
	}{
		{
			desc:          "skip validation of /providers/applications.core/operations",
			method:        http.MethodGet,
			route:         skipRoute,
			apiVersion:    "2022-03-15-privatepreview",
			url:           "http://localhost:8080/providers/applications.core/operations",
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid get-environment with azure resource id",
			method:        http.MethodGet,
			route:         envRoute,
			apiVersion:    "2022-03-15-privatepreview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:          "valid delete-environment with azure resource id",
			method:        http.MethodDelete,
			route:         envRoute,
			apiVersion:    "2022-03-15-privatepreview",
			url:           resourceIDUrl,
			responseCode:  http.StatusAccepted,
			validationErr: nil,
		},
		{
			desc:            "invalid put-environment with invalid api-version",
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-06-20-privatepreview", // unsupported api version
			contentFilePath: "put-environments-valid.json",
			url:             resourceIDUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "InvalidApiVersionParameter",
					Message: "API version '2022-06-20-privatepreview' for type 'applications.core/environments' is not supported.",
				},
			},
		},
		{
			desc:            "invalid put-environment with missing location property bag",
			method:          http.MethodPut,
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
			desc:            "invalid put-environment with invalid enum item",
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-enum.json",
			url:             resourceIDUrl,
			responseCode:    400,
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "HttpRequestPayloadAPISpecValidationFailed",
					Target:  "applications.core/environments/env0",
					Message: "HTTP request payload failed validation against API specification with one or more errors. Please see details for more information.",
					Details: []armerrors.ErrorDetails{
						{
							Code:    "InvalidProperties",
							Message: "$.properties.compute.kind in body should be one of [kubernetes]",
						},
					},
				},
			},
		},
		{
			desc:            "invalid put-environment with invalid json doc",
			method:          http.MethodPut,
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

			// Add API validator middleware
			r.Use(APIValidator(l, []string{skipRoute}))
			r.Path(tc.route).Methods(tc.method).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

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

			require.Equal(t, tc.responseCode, w.Result().StatusCode)

			if w.Result().StatusCode != http.StatusAccepted {
				armErr := &armerrors.ErrorResponse{}
				err := json.Unmarshal(w.Body.Bytes(), armErr)
				require.NoError(t, err)
				require.Equal(t, tc.validationErr, armErr)
			}
		})
	}
}
