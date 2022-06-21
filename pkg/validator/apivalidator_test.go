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
	envRoute = "/{rootScope:.*}/providers/applications.core/environments/{environmentName}"
	baseUrl  = "http://localhost:8080/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/radius-test-rg/providers/applications.core/environments/env0"
)

func TestAPIValidator(t *testing.T) {
	// Load OpenAPI Spec for applications.core provider.
	l := NewLoader("applications.core", swagger.SpecFiles)
	err := l.LoadSpec()

	require.NoError(t, err)

	validatorTests := []struct {
		method       string
		route        string
		resourceType string
		apiVersion   string

		contentFilePath string
		url             string
		responseCode    int
		validationErr   *armerrors.ErrorResponse
	}{
		{
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             baseUrl,
			responseCode:    http.StatusAccepted,
			validationErr:   nil,
		},
		{
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-valid.json",
			url:             "http://localhost/invalid/path",
			responseCode:    http.StatusAccepted,
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "BadRequest",
					Message: "Invalid Resource ID: ",
				},
			},
		},
		{
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-06-20-privatepreview", // unsupported api version
			contentFilePath: "put-environments-valid.json",
			url:             baseUrl,
			responseCode:    http.StatusBadRequest,
			validationErr: &armerrors.ErrorResponse{
				Error: armerrors.ErrorDetails{
					Code:    "BadRequest",
					Message: "API version '2022-06-20-privatepreview' for type 'applications.core/environments' is not supported.",
				},
			},
		},
		{
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-location.json",
			url:             baseUrl,
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
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-property.json",
			url:             baseUrl,
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
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-missing2.json",
			url:             baseUrl,
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
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-enum.json",
			url:             baseUrl,
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
			method:          http.MethodPut,
			route:           envRoute,
			apiVersion:      "2022-03-15-privatepreview",
			contentFilePath: "put-environments-invalid-json.json",
			url:             baseUrl,
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
		t.Run(tc.route, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := mux.NewRouter()

			// Add API validator middleware
			r.Use(APIValidator(l, nil))
			r.Path(tc.route).Methods(tc.method).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

			// Load test fixture.
			var body []byte
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
