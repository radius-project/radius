// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cmd

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/radclient"
	"github.com/stretchr/testify/assert"
)

func TestPrettyPrintRPError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		input  error
		expect string
	}{{
		name:   "non-RP error",
		input:  errors.New("not an RP error"),
		expect: "",
	}, {
		name:   "non-RP wrapped error",
		input:  fmt.Errorf("%w", errors.New("not an RP error")),
		expect: "",
	}, {
		name: "RP error",
		input: fmt.Errorf("%w", &radclient.ErrorResponse{
			InnerError: &radclient.ErrorDetail{
				Code:    stringp("code"),
				Message: stringp("message"),
				Details: []*radclient.ErrorDetail{{
					Message: stringp(`{
                      code: "inner-code",
                      message: "inner-message"
                    }`),
				}},
			},
		}),
		expect: `code: code
details:
- message: |
    code: inner-code
    message: inner-message
message: message
`,
	}, {
		name: "RP error, nil detail",
		input: fmt.Errorf("%w", &radclient.ErrorResponse{
			InnerError: &radclient.ErrorDetail{
				Code:    stringp("code"),
				Message: stringp("message"),
				Details: []*radclient.ErrorDetail{nil},
			},
		}),
		expect: `code: code
details:
- null
message: message
`,
	}, {
		name: "RP error, nil message",
		input: fmt.Errorf("%w", &radclient.ErrorResponse{
			InnerError: &radclient.ErrorDetail{
				Code:    stringp("code"),
				Message: stringp("message"),
				Details: []*radclient.ErrorDetail{{
					Code:    stringp("inner-code"),
					Message: nil,
				}},
			},
		}),
		expect: `code: code
details:
- code: inner-code
message: message
`,
	}, {
		name: "RP error, non-JSON message",
		input: fmt.Errorf("%w", &radclient.ErrorResponse{
			InnerError: &radclient.ErrorDetail{
				Code:    stringp("code"),
				Message: stringp("message"),
				Details: []*radclient.ErrorDetail{{
					Message: stringp("I am not JSON"),
				}},
			},
		}),
		expect: `code: code
details:
- message: |
    I am not JSON
message: message
`,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expect, prettyPrintRPError(tc.input))
		})
	}
}

func stringp(s string) *string {
	return &s
}
