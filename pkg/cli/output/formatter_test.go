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

package output

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_NewFormatter(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		expectType  string
		expectError bool
	}{
		{
			name:       "json returns JSONFormatter",
			format:     "json",
			expectType: "*output.JSONFormatter",
		},
		{
			name:       "table returns TableFormatter",
			format:     "table",
			expectType: "*output.TableFormatter",
		},
		{
			name:       "plain-text returns TableFormatter",
			format:     "plain-text",
			expectType: "*output.TableFormatter",
		},
		{
			name:        "unsupported format returns error",
			format:      "xml",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter, err := NewFormatter(tt.format)
			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, formatter)
			} else {
				require.NoError(t, err)
				require.IsType(t, getFormatterForType(tt.expectType), formatter)
			}
		})
	}
}

// getFormatterForType returns a zero-value instance of the given type name for type assertion.
func getFormatterForType(typeName string) Formatter {
	switch typeName {
	case "*output.JSONFormatter":
		return &JSONFormatter{}
	case "*output.TableFormatter":
		return &TableFormatter{}
	default:
		return nil
	}
}
