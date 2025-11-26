/*
Copyright 2026 The Radius Authors.

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

package installer

import (
	"strings"
	"testing"
)

func TestIsValidVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		valid   bool
	}{
		{name: "simple", version: "1.2.3", valid: true},
		{name: "pre", version: "1.2.3-beta.1", valid: true},
		{name: "build", version: "1.2.3+build", valid: true},
		{name: "missing patch", version: "1.2", valid: false},
		{name: "garbage", version: "abc", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidVersion(tt.version); got != tt.valid {
				t.Fatalf("IsValidVersion(%q) = %v, want %v", tt.version, got, tt.valid)
			}
		})
	}
}

func TestIsValidChecksum(t *testing.T) {
	tests := []struct {
		name     string
		checksum string
		valid    bool
	}{
		{name: "prefixed sha", checksum: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", valid: true},
		{name: "bare sha", checksum: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", valid: true},
		{name: "wrong length", checksum: "abc", valid: false},
		{name: "wrong chars", checksum: "sha256:xyz123", valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidChecksum(tt.checksum); got != tt.valid {
				t.Fatalf("IsValidChecksum(%q) = %v, want %v", tt.checksum, got, tt.valid)
			}
		})
	}
}

func TestValidateVersionForPath(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
		errMsg  string
	}{
		// Valid versions
		{name: "simple semver", version: "1.6.4", wantErr: false},
		{name: "semver with prerelease", version: "1.6.4-beta.1", wantErr: false},
		{name: "semver with build", version: "1.6.4+build", wantErr: false},
		{name: "custom tag latest", version: "latest", wantErr: false},
		{name: "custom tag stable", version: "stable", wantErr: false},
		{name: "version with dash", version: "v1-6-4", wantErr: false},

		// Invalid versions - path traversal attacks
		// Note: Versions with "/" are caught by path separator check first
		{name: "path traversal basic", version: "../../../etc", wantErr: true, errMsg: "path separator"},
		{name: "path traversal with version", version: "1.0.0/../../../etc", wantErr: true, errMsg: "path separator"},
		{name: "double dot alone", version: "..", wantErr: true, errMsg: "path traversal"},
		{name: "consecutive dots allowed", version: "1..2", wantErr: false},

		// Invalid versions - path separators
		{name: "forward slash", version: "1.6/4", wantErr: true, errMsg: "path separator"},
		{name: "backslash", version: "1.6\\4", wantErr: true, errMsg: "path separator"},
		{name: "absolute path unix", version: "/etc/passwd", wantErr: true, errMsg: "path separator"},
		{name: "absolute path windows", version: "C:\\Windows", wantErr: true, errMsg: "path separator"},

		// Invalid versions - empty
		{name: "empty string", version: "", wantErr: true, errMsg: "required"},
		{name: "whitespace only", version: "   ", wantErr: true, errMsg: "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersionForPath(tt.version)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ValidateVersionForPath(%q) expected error, got nil", tt.version)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Fatalf("ValidateVersionForPath(%q) error = %v, want error containing %q", tt.version, err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateVersionForPath(%q) unexpected error: %v", tt.version, err)
				}
			}
		})
	}
}
