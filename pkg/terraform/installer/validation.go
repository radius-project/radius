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
	"fmt"
	"regexp"
	"strings"
)

var (
	versionRe  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+].*)?$`)
	checksumRe = regexp.MustCompile(`^(?i:(sha256:)?[a-f0-9]{64})$`)
)

// IsValidVersion returns true if the version string is in a simple semver-like format.
func IsValidVersion(v string) bool {
	return versionRe.MatchString(v)
}

// IsValidChecksum returns true if the checksum string appears to be a sha256 hex string with optional prefix.
func IsValidChecksum(c string) bool {
	return checksumRe.MatchString(c)
}

// ValidateVersionForPath ensures the version is safe to use in filesystem paths.
// Returns error if version contains path traversal or separator characters.
// NOTE: This validates path safety, not semver compliance - "latest" or custom tags are allowed.
func ValidateVersionForPath(version string) error {
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("version is required")
	}
	// Check for path traversal patterns: "../", "/..", "..\", "\..", or standalone ".."
	// Note: We check for path separators separately, so here we only need to check
	// for ".." as a standalone value (which would be the entire version string)
	if version == ".." {
		return fmt.Errorf("invalid version: contains path traversal sequence")
	}
	if strings.ContainsAny(version, "/\\") {
		return fmt.Errorf("invalid version: contains path separator")
	}
	// Only validate path safety, not semver format - allow "latest", custom tags, etc.
	return nil
}
