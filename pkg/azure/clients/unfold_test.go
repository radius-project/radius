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
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
)

func TestUnfoldErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  v1.ErrorDetails
		expect v1.ErrorDetails
	}{
		{
			name: "no msg",
			input: v1.ErrorDetails{
				Code: "code",
			},
			expect: v1.ErrorDetails{
				Code: "code",
			},
		},
		{
			name: "wrapped none",
			input: v1.ErrorDetails{
				Code:    "code",
				Message: "message",
			},
			expect: v1.ErrorDetails{
				Code:    "code",
				Message: "message",
			},
		},
		{
			name: "wrapped once",
			input: v1.ErrorDetails{
				Code:    "code",
				Message: `{"error": {"code": "inner-code", "message": "inner-message" }}`,
			},
			expect: v1.ErrorDetails{
				Code: "code",
				Details: []v1.ErrorDetails{{
					Code:    "inner-code",
					Message: "inner-message",
				}},
			},
		},
		{
			name: "wrapped twice", // This case does really happens in `rad deploy` calls.
			input: v1.ErrorDetails{
				Code: "code",
				Message: `
                          {
                            "error": {
                              "code": "first-level",
                              "message": "{\"error\":{\"code\": \"second-level\", \"message\": \"I kid you not\"}}"
                            }
                          }`,
			},
			expect: v1.ErrorDetails{
				Code: "code",
				Details: []v1.ErrorDetails{{
					Code: "first-level",
					Details: []v1.ErrorDetails{{
						Code:    "second-level",
						Message: "I kid you not",
					}},
				}},
			},
		},
		{
			name: "details[*].message wrapped once",
			input: v1.ErrorDetails{
				Code:    "DownstreamEndpointError",
				Message: "Please refer to additional info for details",
				Details: []v1.ErrorDetails{{
					Code:    "Downstream",
					Message: `{"error": {"code": "BadRequest", "message": "Validation error" }}`,
					Target:  "",
				}}},
			expect: v1.ErrorDetails{
				Code:    "DownstreamEndpointError",
				Message: "Please refer to additional info for details",
				Details: []v1.ErrorDetails{{
					Code: "Downstream",
					Details: []v1.ErrorDetails{{
						Code:    "BadRequest",
						Message: "Validation error",
					}},
				}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, UnfoldErrorDetails(&tc.input)); diff != "" {
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
	}{
		{
			name:   "empty",
			input:  azure.ServiceError{},
			expect: ServiceError{},
		},
		{
			name: "nested once",
			input: azure.ServiceError{
				Details: []map[string]any{{
					"code":    to.Ptr("DownstreamEndpointError"),
					"message": `{"error": { "code": "BadRequest" }}`,
				}},
			},
			expect: ServiceError{
				Details: []*v1.ErrorDetails{{
					Code: "DownstreamEndpointError",
					Details: []v1.ErrorDetails{{
						Code: "BadRequest",
					}},
				}},
			},
		},
		// {
		// 	name: "nested once, but can't parse using roundTripJSON",
		// 	input: azure.ServiceError{
		// 		Details: []map[string]interface{}{{
		// 			"code":           to.StringPtr("DownstreamEndpointError"),
		// 			"message":        `{"error": { "code": "BadRequest" }}`,
		// 			"additionalInfo": "bad-info, can't parse",
		// 		}},
		// 	},
		// 	expect: ServiceError{
		// 		Details: []*v1.ErrorDetails{{
		// 			Code: "DownstreamEndpointError",
		// 			Details: []v1.ErrorDetails{{
		// 				Code: "BadRequest",
		// 			}},
		// 		}},
		// 	},
		// },
	} {
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
		expect *v1.ErrorDetails
	}{
		{
			name:  "generic err",
			input: errors.New("generic err"),
		},
		{
			name:  "wrapped generic err",
			input: fmt.Errorf("%w", errors.New("generic err")),
		},
		// {
		// 	name: "wrapped *v1.ErrorResponse",
		// 	input: fmt.Errorf("%w", &v1.ErrorResponse{
		// 		Error: v1.ErrorDetails{
		// 			Code:    "code",
		// 			Message: "message",
		// 		}}),
		// 	expect: &v1.ErrorDetails{
		// 		Code:    "code",
		// 		Message: "message",
		// 	},
		// },
	} {
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
	}{
		{
			name:  "generic err",
			input: errors.New("generic err"),
		},
		{
			name:  "wrapped generic err",
			input: fmt.Errorf("%w", errors.New("generic err")),
		},
		{
			name: "nested once",
			input: &azure.ServiceError{
				Details: []map[string]any{{
					"code":    to.Ptr("DownstreamEndpointError"),
					"message": `{"error": { "code": "BadRequest" }}`,
				}},
			},
			expect: &ServiceError{
				Details: []*v1.ErrorDetails{{
					Code: "DownstreamEndpointError",
					Details: []v1.ErrorDetails{{
						Code: "BadRequest",
					}},
				}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.expect, TryUnfoldServiceError(tc.input)); diff != "" {
				t.Errorf("UnfoldErrorDetails: (-want,+got): %v", diff)
			}
		})
	}
}
