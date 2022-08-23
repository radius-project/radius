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
	"github.com/google/go-cmp/cmp"
	"github.com/project-radius/radius/pkg/rp/armerrors"
)

func TestUnfoldErrorDetails(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  armerrors.ErrorDetails
		expect armerrors.ErrorDetails
	}{
		{
			name: "no msg",
			input: armerrors.ErrorDetails{
				Code: "code",
			},
			expect: armerrors.ErrorDetails{
				Code: "code",
			},
		},
		{
			name: "wrapped none",
			input: armerrors.ErrorDetails{
				Code:    "code",
				Message: "message",
			},
			expect: armerrors.ErrorDetails{
				Code:    "code",
				Message: "message",
			},
		},
		{
			name: "wrapped once",
			input: armerrors.ErrorDetails{
				Code:    "code",
				Message: `{"error": {"code": "inner-code", "message": "inner-message" }}`,
			},
			expect: armerrors.ErrorDetails{
				Code: "code",
				Details: []armerrors.ErrorDetails{{
					Code:    "inner-code",
					Message: "inner-message",
				}},
			},
		},
		{
			name: "wrapped twice", // This case does really happens in `rad deploy` calls.
			input: armerrors.ErrorDetails{
				Code: "code",
				Message: `
                          {
                            "error": {
                              "code": "first-level",
                              "message": "{\"error\":{\"code\": \"second-level\", \"message\": \"I kid you not\"}}"
                            }
                          }`,
			},
			expect: armerrors.ErrorDetails{
				Code: "code",
				Details: []armerrors.ErrorDetails{{
					Code: "first-level",
					Details: []armerrors.ErrorDetails{{
						Code:    "second-level",
						Message: "I kid you not",
					}},
				}},
			},
		},
		{
			name: "details[*].message wrapped once",
			input: armerrors.ErrorDetails{
				Code:    "DownstreamEndpointError",
				Message: "Please refer to additional info for details",
				Details: []armerrors.ErrorDetails{{
					Code:    "Downstream",
					Message: `{"error": {"code": "BadRequest", "message": "Validation error" }}`,
					Target:  "",
				}}},
			expect: armerrors.ErrorDetails{
				Code:    "DownstreamEndpointError",
				Message: "Please refer to additional info for details",
				Details: []armerrors.ErrorDetails{{
					Code: "Downstream",
					Details: []armerrors.ErrorDetails{{
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
				Details: []map[string]interface{}{{
					"code":    to.StringPtr("DownstreamEndpointError"),
					"message": `{"error": { "code": "BadRequest" }}`,
				}},
			},
			expect: ServiceError{
				Details: []*armerrors.ErrorDetails{{
					Code: "DownstreamEndpointError",
					Details: []armerrors.ErrorDetails{{
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
		// 		Details: []*armerrors.ErrorDetails{{
		// 			Code: "DownstreamEndpointError",
		// 			Details: []armerrors.ErrorDetails{{
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
		expect *armerrors.ErrorDetails
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
		// 	name: "wrapped *armerrors.ErrorResponse",
		// 	input: fmt.Errorf("%w", &armerrors.ErrorResponse{
		// 		Error: armerrors.ErrorDetails{
		// 			Code:    "code",
		// 			Message: "message",
		// 		}}),
		// 	expect: &armerrors.ErrorDetails{
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
				Details: []map[string]interface{}{{
					"code":    to.StringPtr("DownstreamEndpointError"),
					"message": `{"error": { "code": "BadRequest" }}`,
				}},
			},
			expect: &ServiceError{
				Details: []*armerrors.ErrorDetails{{
					Code: "DownstreamEndpointError",
					Details: []armerrors.ErrorDetails{{
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
