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

package storeutil

import (
	"strings"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	ScopePrefix    = "scope"
	ResourcePrefix = "resource"
)

// # Function Explanation
//
// ExtractStorageParts extracts the main components of the resource id in a way that easily
// supports our storage abstraction. Returns a tuple of the (prefix, rootScope, routingScope, resourceType)
func ExtractStorageParts(id resources.ID) (string, string, string, string) {
	if id.IsScope() {
		// For a scope we encode the last scope segment as the routing scope, and the previous
		// scope segments as the root scope. This gives us the most desirable behavior for
		// queries and recursion.

		prefix := ScopePrefix
		rootScope := NormalizePart(id.Truncate().RootScope())

		last := resources.ScopeSegment{}
		if len(id.ScopeSegments()) > 0 {
			last = id.ScopeSegments()[len(id.ScopeSegments())-1]
		}
		routingScope := NormalizePart(last.Type + resources.SegmentSeparator + last.Name)
		resourceType := strings.ToLower(last.Type)

		return prefix, rootScope, routingScope, resourceType
	} else {
		prefix := ResourcePrefix
		rootScope := NormalizePart(id.RootScope())
		routingScope := NormalizePart(id.RoutingScope())
		resourceType := strings.ToLower(id.Type())

		return prefix, rootScope, routingScope, resourceType
	}
}

// # Function Explanation
//
// IDMatchesQuery checks if the given ID matches the given query.
func IDMatchesQuery(id resources.ID, query store.Query) bool {
	prefix, rootScope, routingScope, resourceType := ExtractStorageParts(id)
	if query.IsScopeQuery && !strings.EqualFold(prefix, ScopePrefix) {
		return false
	} else if !query.IsScopeQuery && !strings.EqualFold(prefix, ResourcePrefix) {
		return false
	}

	if query.ScopeRecursive && !strings.HasPrefix(rootScope, NormalizePart(query.RootScope)) {
		return false
	} else if !query.ScopeRecursive && !strings.EqualFold(rootScope, NormalizePart(query.RootScope)) {
		return false
	}

	if query.RoutingScopePrefix != "" && !strings.HasPrefix(routingScope, NormalizePart(query.RoutingScopePrefix)) {
		return false
	}

	if query.ResourceType != "" && !strings.EqualFold(resourceType, query.ResourceType) {
		return false
	}

	return true
}

// # Function Explanation
//
// NormalizePart takes in a string and returns a normalized version of it with a prefix and suffix segment separator.
func NormalizePart(part string) string {
	if len(part) == 0 {
		return ""
	}
	if !strings.HasPrefix(part, resources.SegmentSeparator) {
		part = resources.SegmentSeparator + part
	}
	if !strings.HasSuffix(part, resources.SegmentSeparator) {
		part = part + resources.SegmentSeparator
	}

	return strings.ToLower(part)
}
