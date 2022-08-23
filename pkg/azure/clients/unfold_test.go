// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/google/go-cmp/cmp"
	"github.com/project-radius/radius/pkg/azure/radclient"
)

func TestUnfoldErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  radclient.ErrorDetail
		expect radclient.ErrorDetail
	}{{
		name: "no msg",
		input: radclient.ErrorDetail{
			Code: to.StringPtr("code"),
		},
		expect: radclient.ErrorDetail{
			Code: to.StringPtr("code"),
		},
	}, {
		name: "wrapped none",
		input: radclient.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
		expect: radclient.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
	}, {
		name: "wrapped once",
		input: radclient.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr(`{"error": {"code": "inner-code", "message": "inner-message" }}`),
		},
		expect: radclient.ErrorDetail{
			Code: to.StringPtr("code"),
			Details: []*radclient.ErrorDetail{{
				Code:    to.StringPtr("inner-code"),
				Message: to.StringPtr("inner-message"),
			}},
		},
	}, {
		name: "wrapped twice", // This case does really happens in `rad deploy` calls.
		input: radclient.ErrorDetail{
			Code: to.StringPtr("code"),
			Message: to.StringPtr(`
                          {
                            "error": {
                              "code": "first-level",
                              "message": "{\"error\":{\"code\": \"second-level\", \"message\": \"I kid you not\"}}"
                            }
                          }`),
		},
		expect: radclient.ErrorDetail{
			Code: to.StringPtr("code"),
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("first-level"),
				Details: []*radclient.ErrorDetail{{
					Code:    to.StringPtr("second-level"),
					Message: to.StringPtr("I kid you not"),
				}},
			}},
		},
	}, {
		name: "details[*].message wrapped once",
		input: radclient.ErrorDetail{
			Code:    to.StringPtr("DownstreamEndpointError"),
			Message: to.StringPtr("Please refer to additional info for details"),
			Details: []*radclient.ErrorDetail{{
				Code:    to.StringPtr("Downstream"),
				Message: to.StringPtr(`{"error": {"code": "BadRequest", "message": "Validation error" }}`),
				Target:  to.StringPtr(""),
			}}},
		expect: radclient.ErrorDetail{
			Code:    to.StringPtr("DownstreamEndpointError"),
			Message: to.StringPtr("Please refer to additional info for details"),
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("Downstream"),
				Details: []*radclient.ErrorDetail{{
					Code:    to.StringPtr("BadRequest"),
					Message: to.StringPtr("Validation error"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, *UnfoldErrorDetails(&tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetails: (-want,+got): %v", diff)
			}
		})
	}
}

func TestUnfoldServiceError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  azure.ServiceError
		expect ServiceError
	}{{
		name:   "empty",
		input:  azure.ServiceError{},
		expect: ServiceError{},
	}, {
		name: "nested once",
		input: azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":    to.StringPtr("DownstreamEndpointError"),
				"message": `{"error": { "code": "BadRequest" }}`,
			}},
		},
		expect: ServiceError{
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
			}},
		},
	}, {
		name: "message with invalid json format persists message",
		input: azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":    to.StringPtr("BadRequest"),
				"message": `{ "code": "BadRequest", "message": "Resource name db in request-uri does not match Resource name db2 in request-body.\\r\\nActivityId: 1ca0e394-3e49-4498-ba93-5a7785f6dc0b, Microsoft.Azure.Documents.Common/2.14.0"}`,
			}},
		},
		expect: ServiceError{
			Details: []*radclient.ErrorDetail{{
				Code:    to.StringPtr("BadRequest"),
				Message: to.StringPtr(`{ "code": "BadRequest", "message": "Resource name db in request-uri does not match Resource name db2 in request-body.\\r\\nActivityId: 1ca0e394-3e49-4498-ba93-5a7785f6dc0b, Microsoft.Azure.Documents.Common/2.14.0"}`),
			}},
		},
	}, {
		name: "message without json still persists message",
		input: azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":    to.StringPtr("BadRequest"),
				"message": "test",
			}},
		},
		expect: ServiceError{
			Details: []*radclient.ErrorDetail{{
				Code:    to.StringPtr("BadRequest"),
				Message: to.StringPtr("test"),
			}},
		},
	}, {
		name: "nested once, but can't parse using roundTripJSON",
		input: azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":           to.StringPtr("DownstreamEndpointError"),
				"message":        `{"error": { "code": "BadRequest" }}`,
				"additionalInfo": "bad-info, can't parse",
			}},
		},
		expect: ServiceError{
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, *UnfoldServiceError(&tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetails: (-want,+got): %v", diff)
			}
		})
	}
}

func TestTryUnfoldErrorResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  error
		expect *radclient.ErrorDetail
	}{{
		name:  "generic err",
		input: errors.New("generic err"),
	}, {
		name:  "wrapped generic err",
		input: fmt.Errorf("%w", errors.New("generic err")),
	}, {
		name: "wrapped *radclient.ErrorResponse",
		input: fmt.Errorf("%w", &radclient.ErrorResponse{
			InnerError: &radclient.ErrorDetail{
				Code:    to.StringPtr("code"),
				Message: to.StringPtr("message"),
			}}),
		expect: &radclient.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, TryUnfoldErrorResponse(tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetails: (-want,+got): %v", diff)
			}
		})
	}
}

func TestTryUnfoldServiceError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  error
		expect *ServiceError
	}{{
		name:  "generic err",
		input: errors.New("generic err"),
	}, {
		name:  "wrapped generic err",
		input: fmt.Errorf("%w", errors.New("generic err")),
	}, {
		name: "nested once",
		input: &azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":    to.StringPtr("DownstreamEndpointError"),
				"message": `{"error": { "code": "BadRequest" }}`,
			}},
		},
		expect: &ServiceError{
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, TryUnfoldServiceError(tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetails: (-want,+got): %v", diff)
			}
		})
	}
}
