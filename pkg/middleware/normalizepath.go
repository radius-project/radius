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
	"strings"
)

const (
	PlanesSegment         = "/planes"
	ResourceGroupsSegment = "/resourcegroups"
)

// NormalizePath replaces any occurrences of "planes" and "resourcegroups" in the URL path with the correct case
// and preserves the case for the rest of the URL.
// For example, the user could specify the url as /Planes/radius/local/resourceGroups/abc and this
// will translate it to: /planes/radius/local/resourcegroups/abc
func NormalizePath(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		newURL := r.URL.Path
		if planesIndex := strings.Index(strings.ToLower(newURL), PlanesSegment); planesIndex >= 0 {
			newURL = strings.Replace(newURL, newURL[planesIndex:planesIndex+len(PlanesSegment)], PlanesSegment, 1)
		}

		if resourcegroupsIndex := strings.Index(strings.ToLower(newURL), ResourceGroupsSegment); resourcegroupsIndex >= 0 {
			newURL = strings.Replace(newURL, newURL[resourcegroupsIndex:resourcegroupsIndex+len(ResourceGroupsSegment)], ResourceGroupsSegment, 1)
		}
		r.URL.Path = newURL
		next.ServeHTTP(w, r)
	}
	return http.HandlerFunc(fn)
}
