// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package clients

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/radius/pkg/azure/radclientv3"
	"github.com/google/go-cmp/cmp"
)

func TestUnfoldErrorDetailsV3(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  radclientv3.ErrorDetail
		expect radclientv3.ErrorDetail
	}{{
		name: "no msg",
		input: radclientv3.ErrorDetail{
			Code: to.StringPtr("code"),
		},
		expect: radclientv3.ErrorDetail{
			Code: to.StringPtr("code"),
		},
	}, {
		name: "wrapped none",
		input: radclientv3.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
		expect: radclientv3.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
	}, {
		name: "wrapped once",
		input: radclientv3.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr(`{"error": {"code": "inner-code", "message": "inner-message" }}`),
		},
		expect: radclientv3.ErrorDetail{
			Code: to.StringPtr("code"),
			Details: []*radclientv3.ErrorDetail{{
				Code:    to.StringPtr("inner-code"),
				Message: to.StringPtr("inner-message"),
			}},
		},
	}, {
		name: "wrapped twice", // This case does really happens in `rad deploy` calls.
		input: radclientv3.ErrorDetail{
			Code: to.StringPtr("code"),
			Message: to.StringPtr(`
                          {
                            "error": {
                              "code": "first-level",
                              "message": "{\"error\":{\"code\": \"second-level\", \"message\": \"I kid you not\"}}"
                            }
                          }`),
		},
		expect: radclientv3.ErrorDetail{
			Code: to.StringPtr("code"),
			Details: []*radclientv3.ErrorDetail{{
				Code: to.StringPtr("first-level"),
				Details: []*radclientv3.ErrorDetail{{
					Code:    to.StringPtr("second-level"),
					Message: to.StringPtr("I kid you not"),
				}},
			}},
		},
	}, {
		name: "details[*].message wrapped once",
		input: radclientv3.ErrorDetail{
			Code:    to.StringPtr("DownstreamEndpointError"),
			Message: to.StringPtr("Please refer to additional info for details"),
			Details: []*radclientv3.ErrorDetail{{
				Code:    to.StringPtr("Downstream"),
				Message: to.StringPtr(`{"error": {"code": "BadRequest", "message": "Validation error" }}`),
				Target:  to.StringPtr(""),
			}}},
		expect: radclientv3.ErrorDetail{
			Code:    to.StringPtr("DownstreamEndpointError"),
			Message: to.StringPtr("Please refer to additional info for details"),
			Details: []*radclientv3.ErrorDetail{{
				Code: to.StringPtr("Downstream"),
				Details: []*radclientv3.ErrorDetail{{
					Code:    to.StringPtr("BadRequest"),
					Message: to.StringPtr("Validation error"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, *UnfoldErrorDetailsV3(&tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetailsV3: (-want,+got): %v", diff)
			}
		})
	}
}

func TestUnfoldServiceErrorV3(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  azure.ServiceError
		expect ServiceErrorV3
	}{{
		name:   "empty",
		input:  azure.ServiceError{},
		expect: ServiceErrorV3{},
	}, {
		name: "nested once",
		input: azure.ServiceError{
			Details: []map[string]interface{}{{
				"code":    to.StringPtr("DownstreamEndpointError"),
				"message": `{"error": { "code": "BadRequest" }}`,
			}},
		},
		expect: ServiceErrorV3{
			Details: []*radclientv3.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclientv3.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
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
		expect: ServiceErrorV3{
			Details: []*radclientv3.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclientv3.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, *UnfoldServiceErrorV3(&tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetailsV3: (-want,+got): %v", diff)
			}
		})
	}
}

func TestTryUnfoldErrorResponseV3(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  error
		expect *radclientv3.ErrorDetail
	}{{
		name:  "generic err",
		input: errors.New("generic err"),
	}, {
		name:  "wrapped generic err",
		input: fmt.Errorf("%w", errors.New("generic err")),
	}, {
		name: "wrapped *radclientv3.ErrorResponseV3",
		input: fmt.Errorf("%w", &radclientv3.ErrorResponse{
			InnerError: &radclientv3.ErrorDetail{
				Code:    to.StringPtr("code"),
				Message: to.StringPtr("message"),
			}}),
		expect: &radclientv3.ErrorDetail{
			Code:    to.StringPtr("code"),
			Message: to.StringPtr("message"),
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, TryUnfoldErrorResponseV3(tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetailsV3: (-want,+got): %v", diff)
			}
		})
	}
}

func TestTryUnfoldServiceErrorV3(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  error
		expect *ServiceErrorV3
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
		expect: &ServiceErrorV3{
			Details: []*radclientv3.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclientv3.ErrorDetail{{
					Code: to.StringPtr("BadRequest"),
				}},
			}},
		},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, TryUnfoldServiceErrorV3(tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetailsV3: (-want,+got): %v", diff)
			}
		})
	}
}
