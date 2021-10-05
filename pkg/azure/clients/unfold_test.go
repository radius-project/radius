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
	"github.com/Azure/radius/pkg/azure/radclient"
	"github.com/google/go-cmp/cmp"
)

func TestUnfoldErrorDetailsV3(t *testing.T) {
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
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
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
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
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
		expect *radclient.ErrorDetail
	}{{
		name:  "generic err",
		input: errors.New("generic err"),
	}, {
		name:  "wrapped generic err",
		input: fmt.Errorf("%w", errors.New("generic err")),
	}, {
		name: "wrapped *radclient.ErrorResponseV3",
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
			Details: []*radclient.ErrorDetail{{
				Code: to.StringPtr("DownstreamEndpointError"),
				Details: []*radclient.ErrorDetail{{
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
