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
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// maxCodeReferenceLength bounds the size of a CodeReference value to keep stored
// data reasonable. The limit comfortably accommodates a deep GitHub blob URL
// with a line anchor.
const maxCodeReferenceLength = 2048

// codeReferenceAnchorRE matches a single-line GitHub-style anchor of the form "L<line>"
// where <line> is a positive integer (no leading zero).
var codeReferenceAnchorRE = regexp.MustCompile(`^L[1-9][0-9]*$`)

// ValidateCodeReference validates the format of a CodeReference value. An empty
// string is treated as unset and is considered valid (the field is optional).
//
// Valid forms:
//   - A repository-root-relative file path using forward slashes, e.g.
//     "src/cache/redis.ts".
//   - The same with an optional GitHub-style single-line anchor appended as a
//     fragment, e.g. "src/cache/redis.ts#L10".
//   - An absolute http:// or https:// URL, e.g.
//     "https://github.com/radius-project/radius/blob/main/pkg/.../service.go#L31".
//
// The value must point to a file (not a directory) and must not contain path
// traversal segments ("." or ".."). Backslashes are not permitted.
func ValidateCodeReference(s string) error {
	if s == "" {
		return nil
	}
	if len(s) > maxCodeReferenceLength {
		return fmt.Errorf("codeReference exceeds the maximum length of %d characters", maxCodeReferenceLength)
	}
	if strings.ContainsRune(s, '\\') {
		return fmt.Errorf("codeReference must use forward slashes only")
	}

	pathPart, anchor, hasAnchor := strings.Cut(s, "#")
	if hasAnchor && !codeReferenceAnchorRE.MatchString(anchor) {
		return fmt.Errorf("codeReference anchor %q must have the form '#L<line>' where <line> is a positive integer", "#"+anchor)
	}

	if strings.HasPrefix(pathPart, "http://") || strings.HasPrefix(pathPart, "https://") {
		return validateCodeReferenceURL(pathPart)
	}

	return validateCodeReferenceRelativePath(pathPart)
}

func validateCodeReferenceURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("codeReference URL is not parseable: %v", err)
	}
	if u.Host == "" {
		return fmt.Errorf("codeReference URL must include a host")
	}
	if u.Path == "" || u.Path == "/" || strings.HasSuffix(u.Path, "/") {
		return fmt.Errorf("codeReference URL must point to a file, not a directory")
	}
	return nil
}

func validateCodeReferenceRelativePath(p string) error {
	if p == "" {
		return fmt.Errorf("codeReference must include a file path")
	}
	if strings.HasPrefix(p, "/") {
		return fmt.Errorf("codeReference must be a repository-root-relative path and must not start with '/'")
	}
	if strings.HasSuffix(p, "/") {
		return fmt.Errorf("codeReference must point to a file, not a directory")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return fmt.Errorf("codeReference must not contain empty path segments")
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("codeReference must not contain '.' or '..' path segments")
		}
	}
	return nil
}
