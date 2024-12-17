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

package databaseutil

import (
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	ScopePrefix    = "scope"
	ResourcePrefix = "resource"
)

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

// IDMatchesQuery checks if the given ID matches the given query.
func IDMatchesQuery(id resources.ID, query database.Query) bool {
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

// ConvertScopeIDToResourceID normalizes the resource ID to be consistent between scopes and resources.
//
// For a resource id that identifies a resource, it is already in normalized form.
//
// - eg: "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/applications/my-app" is already
// normalized.
// - eg: "/planes/radius/local/resourceGroups/my-rg" needs normalization.
func ConvertScopeIDToResourceID(parsed resources.ID) (resources.ID, error) {
	// This function normalizes the resource ID to be consistent between scopes and resources.
	//
	// For a resource id that identifies a resource, it is already in normalized form.
	//
	// eg: "/planes/radius/local/resourceGroups/my-rg/providers/Applications.Core/applications/my-app"
	if parsed.IsResource() {
		return parsed, nil
	}

	// For a resource id that identifies a scope, we truncate the last segement and make it look like a resource by
	// adding a type.
	//
	// eg: "/planes/radius/local/resourceGroups/my-rg" -> "/planes/radius/local/providers/System.Resources/resourceGroups/my-rg"
	//
	// This means that the scope could be looked up using two different forms.
	//
	// - "/planes/radius/local/resourceGroups/my-rg"
	// - "/planes/radius/local/providers/System.Resources/resourceGroups/my-rg"
	//
	// This important because right now the controller code uses the former, but the latter is more useful for storage.
	// Over time we want to eliminate the former and only use the latter by pushing this change into the controllers.
	//
	// There's a fixed set of scopes in Radius/UCP right now but we'd like to avoid maintaining a hardcoded list
	// of scope types.
	//
	// Cases we have to handle:
	// - /planes/azure/<name>
	// - /planes/aws/<name>
	// - /planes/radius/<name>
	// - /planes/radius/<name>/resourceGroups/<name>
	scopes := parsed.ScopeSegments()
	if len(scopes) == 0 {
		return resources.ID{}, fmt.Errorf("invalid resource id: %s", parsed.String())
	}

	switch strings.ToLower(scopes[0].Type) {
	case "azure":
		if len(scopes) == 1 {
			return resources.MustParse(fmt.Sprintf("/planes/providers/System.Azure/planes/%s", scopes[0].Name)), nil
		}

	case "aws":
		if len(scopes) == 1 {
			return resources.MustParse(fmt.Sprintf("/planes/providers/System.AWS/planes/%s", scopes[0].Name)), nil
		}

	case "radius":
		if len(scopes) == 1 {
			return resources.MustParse(fmt.Sprintf("/planes/providers/System.Radius/planes/%s", scopes[0].Name)), nil
		} else if len(scopes) == 2 && strings.EqualFold(scopes[1].Type, "resourceGroups") {
			return resources.MustParse(fmt.Sprintf("/planes/radius/%s/providers/System.Resources/resourceGroups/%s", scopes[0].Name, scopes[1].Name)), nil
		}
	}

	return resources.ID{}, fmt.Errorf("invalid resource id: %s", parsed.String())
}

// ConvertScopeTypeToResourceType normalizes the resource type to be consistent between scopes and resources.
// See comments on ConvertScopeIDToResourceID for full context.
//
// For a resource type that identifies a resource, it is already in normalized form.
//
// - eg: "Applications.Core/applications" is already normalized.
// - eg: "resourceGroups" needs normalization.
func ConvertScopeTypeToResourceType(resourceType string) (string, error) {
	if strings.Contains(resourceType, "/") {
		// Already normalized.
		return resourceType, nil
	}

	switch strings.ToLower(resourceType) {
	case "aws":
		return "System.Aws/planes", nil
	case "azure":
		return "System.Azure/planes", nil
	case "radius":
		return "System.Radius/planes", nil
	case "resourcegroups":
		return "System.Resources/resourceGroups", nil
	}

	return "", fmt.Errorf("invalid resource type: %s", resourceType)
}
