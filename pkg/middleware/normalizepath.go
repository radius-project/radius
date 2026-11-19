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

package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	PlanesSegment         = "/planes"
	ResourceGroupsSegment = "/resourcegroups"
)

// NormalizePath rewrites the first "planes" path segment and the first "resourcegroups" path segment
// to lowercase and preserves every other byte of the URL path.
// For example, the user could specify the url as /Planes/radius/local/resourceGroups/abc and this
// will translate it to: /planes/radius/local/resourcegroups/abc
func NormalizePath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		normalizeURLPath(r.URL)
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}

// normalizeURLPath keeps URL.Path and URL.RawPath consistent. The router matches on RawPath when it is
// set, so when the request carries a non-default escaping, the escaped form is normalized and the
// decoded path is derived from it. Matching on escaped segments keeps an encoded slash (%2F) inside
// a single segment.
func normalizeURLPath(u *url.URL) {
	if u.RawPath != "" {
		if decoded, err := url.PathUnescape(u.RawPath); err == nil && decoded == u.Path {
			rawPath := normalizePath(u.RawPath, url.PathUnescape)
			if path, err := url.PathUnescape(rawPath); err == nil {
				u.RawPath = rawPath
				u.Path = path
				return
			}
		}
	}

	u.Path = normalizePath(u.Path, nil)
}

// normalizePath compares whole segments with ASCII-only case folding so that byte offsets are never
// transferred from a case-mapped copy of the path, which would corrupt paths containing characters
// whose UTF-8 length changes when lowercased. When decode is non-nil, each segment is compared by its
// decoded value.
func normalizePath(path string, decode func(string) (string, error)) string {
	planes := strings.TrimPrefix(PlanesSegment, "/")
	resourceGroups := strings.TrimPrefix(ResourceGroupsSegment, "/")

	segments := strings.Split(path, "/")
	changed := false
	foundPlanes, foundResourceGroups := false, false
	for i, segment := range segments {
		value := segment
		if decode != nil {
			decoded, err := decode(segment)
			if err != nil {
				continue
			}
			value = decoded
		}

		var target string
		switch {
		case !foundPlanes && equalFoldASCII(value, planes):
			foundPlanes = true
			target = planes
		case !foundResourceGroups && equalFoldASCII(value, resourceGroups):
			foundResourceGroups = true
			target = resourceGroups
		default:
			continue
		}

		if segment != target {
			segments[i] = target
			changed = true
		}

		if foundPlanes && foundResourceGroups {
			break
		}
	}

	if !changed {
		return path
	}

	return strings.Join(segments, "/")
}

// equalFoldASCII reports whether s equals lower, an ASCII lowercase string, under ASCII case folding.
func equalFoldASCII(s, lower string) bool {
	if len(s) != len(lower) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}
