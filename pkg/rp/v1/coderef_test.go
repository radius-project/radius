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

package v1

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateCodeReference(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		// Valid cases.
		{name: "empty is valid (optional)", input: ""},
		{name: "relative file path", input: "src/cache/redis.ts"},
		{name: "single-segment file", input: "main.go"},
		{name: "relative path with line anchor", input: "src/cache/redis.ts#L10"},
		{name: "deep relative path", input: "a/b/c/d/e/file.ts#L1"},
		{name: "https URL", input: "https://github.com/radius-project/radius/blob/main/pkg/armrpc/asyncoperation/worker/service.go"},
		{name: "https URL with anchor", input: "https://github.com/radius-project/radius/blob/main/pkg/armrpc/asyncoperation/worker/service.go#L31"},
		{name: "http URL", input: "http://example.com/foo/bar.go"},

		// Path-shape failures.
		{name: "absolute path rejected", input: "/etc/passwd", wantErr: true, errContains: "repository-root-relative"},
		{name: "single dot segment rejected", input: "./foo.ts", wantErr: true, errContains: "'.' or '..'"},
		{name: "double dot segment rejected", input: "../foo.ts", wantErr: true, errContains: "'.' or '..'"},
		{name: "embedded double dot rejected", input: "src/../etc/passwd", wantErr: true, errContains: "'.' or '..'"},
		{name: "trailing slash rejected (directory)", input: "src/cache/", wantErr: true, errContains: "file, not a directory"},
		{name: "backslash rejected", input: "src\\cache\\redis.ts", wantErr: true, errContains: "forward slashes"},
		{name: "empty segment rejected", input: "src//redis.ts", wantErr: true, errContains: "empty path segments"},

		// Anchor failures.
		{name: "missing line number rejected", input: "foo.ts#L", wantErr: true, errContains: "form '#L<line>'"},
		{name: "non-numeric anchor rejected", input: "foo.ts#Label", wantErr: true, errContains: "form '#L<line>'"},
		{name: "zero line rejected", input: "foo.ts#L0", wantErr: true, errContains: "form '#L<line>'"},
		{name: "leading zero rejected", input: "foo.ts#L01", wantErr: true, errContains: "form '#L<line>'"},
		{name: "unknown anchor form rejected", input: "foo.ts#section", wantErr: true, errContains: "form '#L<line>'"},

		// URL failures.
		{name: "URL without host rejected", input: "https:///foo/bar.go", wantErr: true, errContains: "host"},
		{name: "URL ending in slash rejected", input: "https://github.com/radius-project/radius/", wantErr: true, errContains: "file, not a directory"},
		{name: "URL bare host rejected", input: "https://github.com", wantErr: true, errContains: "file, not a directory"},

		// Length.
		{name: "overlong rejected", input: "a/" + strings.Repeat("x", 2048), wantErr: true, errContains: "maximum length"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCodeReference(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errContains != "" {
					require.Contains(t, err.Error(), tc.errContains)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}
