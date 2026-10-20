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

package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	apiv1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/test/radcli"
)

func Test_ErrorContainsAny(t *testing.T) {
	// nestedCLIError verifies that ErrorContainsAny matches a substring that only
	// appears inside the nested ARM error details, not the top-level message.
	nestedCLIError := &radcli.CLIError{
		ErrorResponse: apiv1.ErrorResponse{
			Error: &apiv1.ErrorDetails{
				Code:    "DeploymentFailed",
				Message: "At least one resource deployment operation failed.",
				Details: []*apiv1.ErrorDetails{
					{Code: "Internal", Message: "the marker is buried here"},
				},
			},
		},
	}

	tests := []struct {
		name       string
		err        error
		substrings []string
		expected   bool
	}{
		{name: "nil error", err: nil, substrings: []string{"x"}, expected: false},
		{name: "no substrings", err: errors.New("boom"), substrings: nil, expected: false},
		{name: "matches one of several", err: errors.New("boom"), substrings: []string{"nope", "boom"}, expected: true},
		{name: "no match", err: errors.New("boom"), substrings: []string{"nope"}, expected: false},
		{name: "matches nested ARM detail", err: nestedCLIError, substrings: []string{"buried here"}, expected: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ErrorContainsAny(tc.err, tc.substrings...))
		})
	}
}
