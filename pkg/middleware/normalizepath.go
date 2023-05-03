// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package middleware

import (
	"net/http"
	"strings"
)

const (
	PlanesSegment         = "/planes"
	ResourceGroupsSegment = "/resourcegroups"
)

// NormalizePath is the middelware to normalize the case in planes and resourcegroups segments and preserve the case
// for the rest of the URL
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

		// WORKAROUND: Ignore remote address for telemetry to lower cadinality.
		r.RemoteAddr = ""
	}
	return http.HandlerFunc(fn)
}
