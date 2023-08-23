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

package resources

import (
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/exp/slices"
)

const (
	SegmentSeparator      = "/"
	PlanesSegment         = "planes"
	ProvidersSegment      = "providers"
	ResourceGroupsSegment = "resourcegroups"
	SubscriptionsSegment  = "subscriptions"
	LocationsSegment      = "locations"
	AccountsSegment       = "accounts"
	RegionsSegment        = "regions"

	CoreRPNamespace       = "Applications.Core"
	LinkRPNamespace       = "Applications.Link"
	DatastoresRPNamespace = "Applications.Datastores"
	DaprRPNamespace       = "Applications.Dapr"
	MessagingRPNamespace  = "Applications.Messaging"

	PlaneTypePrefix   = "System.Planes"
	ResourceGroupType = "System.Resources/resourceGroups"
)

var supportedNamespaces = []string{
	CoreRPNamespace,
	LinkRPNamespace,
	DatastoresRPNamespace,
	DaprRPNamespace,
	MessagingRPNamespace,
}

// ID represents an ARM or UCP resource id. ID is immutable once created. Use Parse() or ParseXyz()
// to create IDs and use String() to convert back to strings.
type ID struct {
	id                string
	scopeSegments     []ScopeSegment
	typeSegments      []TypeSegment
	extensionSegments []TypeSegment
}

// ScopeSegment represents one of the root-scope pairs of a resource ID.
type ScopeSegment struct {
	// Type is the type of the scope.
	//
	// Example:
	//	resourceGroup
	//	subscription
	//
	Type string

	// Name is the name of the scope.
	Name string
}

// TypeSegment represents one of the type/name pairs of a resource ID.
type TypeSegment struct {
	// Type one of the segments of a resource type. This will be a namespace/type combo for the first
	// segment, and a simple name for subsequent ones.
	//
	// Example:
	//	Microsoft.Resources/deployment
	//  database
	//
	Type string

	// Name is the name of the resource.
	Name string
}

type KnownType struct {
	Types []TypeSegment
}

// IsEmpty checks if the ID is empty.
func (ri ID) IsEmpty() bool {
	return ri.id == ""
}

// IsScope returns true if the ID represents a named scope (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local
func (ri ID) IsScope() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) == 0 && // Not a type
		len(ri.extensionSegments) == 0 &&
		(len(ri.scopeSegments) == 0 || len(ri.scopeSegments[len(ri.scopeSegments)-1].Name) > 0) // No scope segments or last one is named
}

// IsResource returns true if the ID represents a named resource (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app/providers/Some.Other/type/my-extension
func (ri ID) IsResource() bool {
	if ri.IsEmpty() {
		return false
	}

	if len(ri.extensionSegments) > 0 {
		// Has at least one extension segment, and the last one is named.
		return len(ri.extensionSegments) > 0 && len(ri.extensionSegments[len(ri.extensionSegments)-1].Name) > 0
	}

	// Has type segments and last one is named
	return len(ri.typeSegments) > 0 && len(ri.typeSegments[len(ri.typeSegments)-1].Name) > 0
}

// IsScopeCollection returns true if the ID represents a collection or custom action on a scope.
//
// Example:
//
//	/planes/radius/local/resourceGroups/resources
func (ri ID) IsScopeCollection() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) == 0 && // No type segments
		len(ri.extensionSegments) == 0 && // No extension segments
		len(ri.scopeSegments) > 0 && len(ri.scopeSegments[len(ri.scopeSegments)-1].Name) == 0 // Has scope segments and last one is un-named
}

// IsResourceCollection returns true if the ID represents a collection or custom action on a resource.
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app/providers/Some.Other/type
func (ri ID) IsResourceCollection() bool {
	if ri.IsEmpty() {
		return false
	}

	if len(ri.extensionSegments) > 0 {
		// Has at least one extension segment, and the last one is un-named.
		return len(ri.extensionSegments) > 0 && len(ri.extensionSegments[len(ri.extensionSegments)-1].Name) == 0
	}

	// Has type segments and last one is un-named
	return len(ri.typeSegments) > 0 && len(ri.typeSegments[len(ri.typeSegments)-1].Name) == 0
}

// IsExtensionResource returns true if the ID represents an extension resource.
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app/providers/Some.Other/type/my-extension
func (ri ID) IsExtensionResource() bool {
	// Has at least one extension segment, and the last one is named.
	return len(ri.extensionSegments) > 0 && len(ri.extensionSegments[len(ri.extensionSegments)-1].Name) > 0
}

// IsExtensionCollection returns true if the ID represents a collection or custom action on an extension resource.
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app/providers/Some.Other/type
func (ri ID) IsExtensionCollection() bool {
	// Has at least one extension segment, and the last one is named.
	return len(ri.extensionSegments) > 0 && len(ri.extensionSegments[len(ri.extensionSegments)-1].Name) == 0
}

// IsUCPQualfied checks if the ID has a prefix of SegmentSeparator and PlanesSegment.
func (ri ID) IsUCPQualfied() bool {
	return strings.HasPrefix(ri.id, SegmentSeparator+PlanesSegment)
}

// ScopeSegments gets the slice of root-scope segments.
func (ri ID) ScopeSegments() []ScopeSegment {
	return ri.scopeSegments
}

// TypeSegments gets the slice of type segments.
func (ri ID) TypeSegments() []TypeSegment {
	return ri.typeSegments
}

// ExtensionSegments gets the slice of extension segments.
func (ri ID) ExtensionSegments() []TypeSegment {
	return ri.extensionSegments
}

// This function returns the "id" field of the given ID instance.
func (ri ID) String() string {
	return ri.id
}

// Method FindScope searches through the scopeSegments of the ID instance and returns the Name of the scopeType if found.
func (ri ID) FindScope(scopeType string) string {
	for _, t := range ri.scopeSegments {
		if strings.EqualFold(t.Type, scopeType) {
			return t.Name
		}
	}
	return ""
}

// RootScope returns the root-scope (the part before the first 'providers'), taking into account whether the ID is qualified for UCP or not.
//
// For an exension resource the root scope is the same as its parent resource's root scope.
//
// Examples:
//
//	/subscriptions/{guid}/resourceGroups/cool-group
//	/planes/radius/local/resourceGroups/cool-group
func (ri ID) RootScope() string {
	segments := []string{}
	for _, t := range ri.scopeSegments {
		segments = append(segments, t.Type)
		if t.Name != "" {
			segments = append(segments, t.Name)
		}
	}

	joined := strings.Join(segments, SegmentSeparator)
	if ri.IsUCPQualfied() {
		return SegmentSeparator + PlanesSegment + SegmentSeparator + joined
	}

	return SegmentSeparator + joined
}

// PlaneScope returns plane or subscription scope without resourceGroup
//
// Examples:
//
//	/subscriptions/{guid}
//	/planes/radius/local
func (ri ID) PlaneScope() string {
	segments := []string{}
	for _, t := range ri.scopeSegments {
		if !strings.EqualFold(t.Type, ResourceGroupsSegment) {
			segments = append(segments, t.Type)
			if t.Name != "" {
				segments = append(segments, t.Name)
			}
			break
		}
	}

	joined := strings.Join(segments, SegmentSeparator)
	if ri.IsUCPQualfied() {
		return SegmentSeparator + PlanesSegment + SegmentSeparator + joined
	}

	return SegmentSeparator + joined
}

// ProviderNamespace returns the namespace of the resource provider. Will be empty if the resource ID
// is empty or refers to a scope.
//
// Examples:
//
//	Applications.Core
func (ri ID) ProviderNamespace() string {
	if len(ri.extensionSegments) > 0 {
		segments := strings.Split(ri.extensionSegments[0].Type, SegmentSeparator)
		return segments[0]
	}

	if len(ri.typeSegments) > 0 {
		segments := strings.Split(ri.typeSegments[0].Type, SegmentSeparator)
		return segments[0]
	}

	return ""
}

// IsRadiusRPResource checks if the given ID is a supported Radius resource.
func (ri ID) IsRadiusRPResource() bool {
	return slices.Contains(supportedNamespaces, ri.ProviderNamespace())
}

// PlaneNamespace returns the plane part of the UCP ID, or an empty string if the ID is not UCP qualified.
//
// Examples:
//
//	radius/local
func (ri ID) PlaneNamespace() string {
	if !ri.IsUCPQualfied() {
		return ""
	}

	scopeSegment := ri.ScopeSegments()[0]
	keys := []string{
		scopeSegment.Type,
		scopeSegment.Name,
	}
	return strings.Join(keys, "/")
}

// RoutingScope returns the routing-scope (the part after 'providers') - it is composed of the type and name segments of the ID instance.
//
// Examples:
//
//	Applications.Core/applications/my-app
func (ri ID) RoutingScope() string {
	segments := []string{}

	if len(ri.extensionSegments) > 0 {
		for _, t := range ri.extensionSegments {
			segments = append(segments, t.Type)
			if t.Name != "" {
				segments = append(segments, t.Name)
			}
		}
	} else {
		for _, t := range ri.typeSegments {
			segments = append(segments, t.Type)
			if t.Name != "" {
				segments = append(segments, t.Name)
			}
		}
	}

	return strings.Join(segments, SegmentSeparator)
}

// ParentResource returns the parent resource of the resource ID, or an empty string if the ID is a scope or non-extension resource.
//
// Example:
//
//	/planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/application/my-app/providers/Applications.Core/someExtensionType/my-extension
//	=> /planes/radius/local/resourceGroups/cool-group/providers/Applications.Core/application/my-app
func (ri ID) ParentResource() string {
	if len(ri.extensionSegments) == 0 {
		return ""
	}

	if ri.IsUCPQualfied() {
		return MakeUCPID(ri.scopeSegments, ri.typeSegments, nil)
	} else {
		return MakeRelativeID(ri.scopeSegments, ri.typeSegments, nil)
	}
}

// Type returns the fully-qualified resource type of a ResourceID, or an empty string if the type cannot be determined.
func (ri ID) Type() string {
	if len(ri.extensionSegments) > 0 {
		types := make([]string, len(ri.extensionSegments))
		for i, t := range ri.extensionSegments {
			types[i] = t.Type
		}
		return strings.Join(types, SegmentSeparator)
	}

	if len(ri.typeSegments) > 0 {
		types := make([]string, len(ri.typeSegments))
		for i, t := range ri.typeSegments {
			types[i] = t.Type
		}
		return strings.Join(types, SegmentSeparator)
	}

	// Add a special case for the planes/resourcegroups resource
	if len(ri.scopeSegments) == 1 {
		// This is a plane resource
		return PlaneTypePrefix + SegmentSeparator + ri.scopeSegments[0].Type
	} else if len(ri.scopeSegments) == 2 && strings.EqualFold(ri.scopeSegments[1].Type, "resourcegroups") && !ri.IsScopeCollection() {
		// This is a resource group resource
		return ResourceGroupType
	}
	return ""
}

// QualifiedName gets the fully-qualified resource name (eg. `radiusv3/myapp/mycontainer`) by joining the type segments with the SegmentSeparator.
func (ri ID) QualifiedName() string {
	names := []string{}
	if len(ri.extensionSegments) > 0 {
		for _, t := range ri.extensionSegments {
			if t.Name != "" {
				names = append(names, t.Name)
			}
		}

	} else if len(ri.typeSegments) > 0 {
		for _, t := range ri.typeSegments {
			if t.Name != "" {
				names = append(names, t.Name)
			}
		}
	} else if len(ri.scopeSegments) > 0 {
		for _, t := range ri.scopeSegments {
			if t.Name != "" {
				names = append(names, t.Name)
			}
		}
	}

	if len(names) == 0 {
		return ""
	}

	return strings.Join(names, SegmentSeparator)
}

// Name gets the resource or scope name.
func (ri ID) Name() string {
	if len(ri.extensionSegments) > 0 {
		return ri.extensionSegments[len(ri.extensionSegments)-1].Name
	}

	if len(ri.typeSegments) > 0 {
		return ri.typeSegments[len(ri.typeSegments)-1].Name
	}

	if len(ri.scopeSegments) > 0 {
		return ri.scopeSegments[len(ri.scopeSegments)-1].Name
	}

	return ""
}

// ValidateResourceType validates that the resource ID type segment matches the expected type.
func (ri ID) ValidateResourceType(t KnownType) error {
	if len(ri.typeSegments) != len(t.Types) {
		return invalidType(ri.id)
	}

	for i, rt := range t.Types {
		// Mismatched type
		if !strings.EqualFold(rt.Type, ri.typeSegments[i].Type) {
			return invalidType(ri.id)
		}

		// A collection was expected and this has a name.
		if rt.Name == "" && ri.typeSegments[i].Name != "" {
			return invalidType(ri.id)
		}

		// A resource was expected and this is a collection.
		if rt.Name != "" && ri.typeSegments[i].Name == "" {
			return invalidType(ri.id)
		}
	}

	return nil
}

func invalidType(id string) error {
	return fmt.Errorf("resource '%v' does not match the expected resource type", id)
}

// Append appends a resource type segment to the ID and returns the resulting ID. If the ID is UCP qualified, it will
// return a UCP qualified ID, otherwise it will return a relative ID.
func (ri ID) Append(resourceType TypeSegment) ID {
	typeSegments := ri.typeSegments
	extensionSegments := ri.extensionSegments
	if len(ri.extensionSegments) > 0 {
		extensionSegments = append(extensionSegments, resourceType)
	} else {
		typeSegments = append(typeSegments, resourceType)
	}

	if ri.IsUCPQualfied() {
		result, err := Parse(MakeUCPID(ri.scopeSegments, typeSegments, extensionSegments))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	} else {
		result, err := Parse(MakeRelativeID(ri.scopeSegments, typeSegments, extensionSegments))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	}
}

// Truncate removes the last type/name pair for a resource id or scope id. Calling truncate on a top level resource or scope has no effect.
func (ri ID) Truncate() ID {
	scopeSegments := ri.scopeSegments
	typeSegments := ri.typeSegments
	extensionSegments := ri.extensionSegments

	if len(ri.extensionSegments) > 1 {
		extensionSegments = extensionSegments[0 : len(extensionSegments)-1]
	} else if len(ri.extensionSegments) == 1 {
		// Do nothing
		return ri
	} else if len(ri.typeSegments) > 1 {
		typeSegments = typeSegments[0 : len(typeSegments)-1]
	} else if len(ri.typeSegments) == 1 {
		// Do nothing
		return ri
	} else if len(ri.scopeSegments) >= 1 {
		// Allow the last scope to be truncated. An empty ID is still a "scope".
		scopeSegments = scopeSegments[0 : len(scopeSegments)-1]
	}

	if ri.IsUCPQualfied() {
		result, err := Parse(MakeUCPID(scopeSegments, typeSegments, extensionSegments))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	} else {
		result, err := Parse(MakeRelativeID(scopeSegments, typeSegments, extensionSegments))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	}
}

// ParseByMethod is a helper function to extract the custom actions from the id.
// If there is a custom action in the request, then the method will be POST. To be able
// to get the proper type, we need to remove the custom action from the id.
func ParseByMethod(id string, method string) (ID, error) {
	parsedID, err := Parse(id)
	if err != nil {
		return ID{}, err
	}

	if method == http.MethodPost {
		parsedID = parsedID.Truncate()
	}

	return parsedID, nil
}

// ParseScope returns a parsed resource ID if the ID represents a named scope (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1
func ParseScope(id string) (ID, error) {
	parsed, err := Parse(id)
	if err != nil {
		return ID{}, err
	}

	if !parsed.IsScope() {
		return ID{}, fmt.Errorf("%q is a valid resource id but does not refer to a scope", id)
	}

	return parsed, err
}

// ParseResource returns a parsed resource ID if the ID represents a named resource (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app
func ParseResource(id string) (ID, error) {
	parsed, err := Parse(id)
	if err != nil {
		return ID{}, err
	}

	if !parsed.IsResource() {
		return ID{}, fmt.Errorf("%q is a valid resource id but does not refer to a resource", id)
	}

	return parsed, err
}

// Parse parses a resource ID. Parse will parse ALL valid resource IDs in the most permissive way.
// Most code should use a more specific function like ParseResource to parse the specific kind of ID
// they want to handle.
func Parse(id string) (ID, error) {
	original := id
	// We require the leading / for all IDs, and tolerate a trailing /.
	//
	// We block // for explicitly for security reasons so we can avoid a reflected redirect attack.
	// A URL path that contains `//example.com` could end up in the Location header and result in an open
	// redirect.
	if !strings.HasPrefix(id, SegmentSeparator) || strings.HasPrefix(id, SegmentSeparator+SegmentSeparator) {
		return ID{}, invalid(original)
	}

	// trim the leading and ending / so we don't end up with an empty segment - we disallow
	// empty segments in the middle of the string
	id = strings.TrimPrefix(id, SegmentSeparator)
	id = strings.TrimSuffix(id, SegmentSeparator)

	isUCPQualified := false
	if strings.EqualFold(id, PlanesSegment) {
		isUCPQualified = true

		// We don't need to process the planes segment while parsing.
		id = strings.TrimPrefix(id, PlanesSegment)
	} else if strings.HasPrefix(id, PlanesSegment+SegmentSeparator) {
		isUCPQualified = true

		// We don't need to process the planes segment while parsing.
		id = strings.TrimPrefix(id, PlanesSegment+SegmentSeparator)
	}

	// Handle trivial case
	if id == "" {
		normalized := ""
		if isUCPQualified {
			normalized = MakeUCPID(nil, nil, nil)
		} else {
			normalized = MakeRelativeID(nil, nil, nil)
		}
		return ID{
			id: normalized,
		}, nil
	}

	// Check up front for empty segments
	segments := strings.Split(id, SegmentSeparator)
	for _, s := range segments {
		if s == "" {
			return ID{}, invalid(original)
		}
	}

	// Parse scopes - iterate until we get to "providers"
	//
	// Each id has a 'scope' portion and an optional 'resource' followed by an optional 'extension'.
	// The '/providers/' segment is the delimiter between these.
	scopes := []ScopeSegment{}

	i := 0
	for i < len(segments) {
		// We're done parsing scopes when we reach the providers segment.
		if strings.ToLower(segments[i]) == ProvidersSegment {
			if len(segments) == i+1 {
				// Last segment is "providers"
				return ID{}, invalid(original)
			}
			i++ // advance past "providers"
			break
		}

		if len(segments)-i < 2 {
			// One non-providers segments remaining, this is a collection.
			//
			// eg: /planes/radius/local/resourceGroups/test-rg/|resources|
			//
			scopes = append(scopes, ScopeSegment{Type: segments[i], Name: ""})
			i += 1
			break
		}

		if strings.ToLower(segments[i+1]) == ProvidersSegment {
			// odd number of non-providers segments inside the root scope followed by 'providers', this is invalid.
			//
			// eg: /planes/radius/local/resourceGroups/test-rg/|resources|/providers/....
			return ID{}, invalid(original)
		}

		scopes = append(scopes, ScopeSegment{Type: segments[i], Name: segments[i+1]})
		i += 2
	}

	// We might not have a "providers" segment at all, if that's the case then this ID refers to
	// a scope.
	if len(segments)-i == 0 {
		normalized := ""
		if isUCPQualified {
			normalized = MakeUCPID(scopes, nil, nil)
		} else {
			normalized = MakeRelativeID(scopes, nil, nil)
		}

		return ID{
			id:            normalized,
			scopeSegments: scopes,
		}, nil
	}

	// Now that're past providers, we're looking for the namespace/type - that is
	// at least 2 segments.
	if len(segments)-i < 2 {
		return ID{}, invalid(original)
	}

	resourceType := TypeSegment{Type: fmt.Sprintf("%s/%s", segments[i], segments[i+1])}
	i += 2

	// We intentionally tolerate a "collection" id that omits the last name segment
	if len(segments)-i > 0 {
		resourceType.Name = segments[i]
		i++
	}
	types := []TypeSegment{resourceType}

	for i < len(segments) {
		// We're done parsing types when we reach the providers segment.
		if strings.ToLower(segments[i]) == ProvidersSegment {
			if len(segments) == i+1 {
				// Last segment is "providers"
				return ID{}, invalid(original)
			}
			i++ // advance past "providers"
			break
		}

		rt := TypeSegment{Type: segments[i]}
		i++

		// check for a resource name
		if len(segments)-i == 0 {
			// This is a collection.
			types = append(types, rt)
			break
		}

		// we have a name - keep parsing
		rt.Name = segments[i]
		i++

		types = append(types, rt)
	}

	if len(segments)-i == 0 {
		normalized := ""
		if isUCPQualified {
			normalized = MakeUCPID(scopes, types, nil)
		} else {
			normalized = MakeRelativeID(scopes, types, nil)
		}

		return ID{
			id:            normalized,
			scopeSegments: scopes,
			typeSegments:  types,
		}, nil
	}

	// If we get here then this is an extension resource. We need to parse another type.

	// Now that're past providers, we're looking for the namespace/type - that is
	// at least 2 segments.
	if len(segments)-i < 2 {
		return ID{}, invalid(original)
	}

	extensionType := TypeSegment{Type: fmt.Sprintf("%s/%s", segments[i], segments[i+1])}
	i += 2

	// We intentionally tolerate a "collection" id that omits the last name segment
	if len(segments)-i > 0 {
		extensionType.Name = segments[i]
		i++
	}
	extensionTypes := []TypeSegment{extensionType}

	for i < len(segments) {
		et := TypeSegment{Type: segments[i]}
		i++

		// check for a resource name
		if len(segments)-i == 0 {
			// This is a collection.
			extensionTypes = append(extensionTypes, et)
			break
		}

		// we have a name - keep parsing
		et.Name = segments[i]
		i++

		extensionTypes = append(extensionTypes, et)
	}

	normalized := ""
	if isUCPQualified {
		normalized = MakeUCPID(scopes, types, extensionTypes)
	} else {
		normalized = MakeRelativeID(scopes, types, extensionTypes)
	}

	return ID{
		id:                normalized,
		scopeSegments:     scopes,
		typeSegments:      types,
		extensionSegments: extensionTypes,
	}, nil
}

func invalid(id string) error {
	return fmt.Errorf("'%s' is not a valid resource id", id)
}

// MakeUCPID creates a fully-qualified UCP resource ID, from the given scopes and resource types.
func MakeUCPID(scopes []ScopeSegment, resourceTypes []TypeSegment, extensionTypes []TypeSegment) string {
	relative := MakeRelativeID(scopes, resourceTypes, extensionTypes)
	if relative == "/" {
		return SegmentSeparator + PlanesSegment
	}

	return SegmentSeparator + PlanesSegment + relative
}

// MakeRelativeID makes a plane-relative resource ID (ARM style) from a slice of ScopeSegment and a variadic of TypeSegment..
func MakeRelativeID(scopes []ScopeSegment, resourceTypes []TypeSegment, extensionTypes []TypeSegment) string {
	segments := []string{}
	for _, scope := range scopes {
		segments = append(segments, scope.Type)
		if scope.Name != "" {
			segments = append(segments, scope.Name)
		}
	}

	if len(resourceTypes) != 0 {
		segments = append(segments, ProvidersSegment)

		for _, rt := range resourceTypes {
			segments = append(segments, rt.Type)
			if rt.Name != "" {
				segments = append(segments, rt.Name)
			}
		}
	}

	if len(extensionTypes) != 0 {
		segments = append(segments, ProvidersSegment)

		for _, et := range extensionTypes {
			segments = append(segments, et.Type)
			if et.Name != "" {
				segments = append(segments, et.Name)
			}
		}
	}

	return SegmentSeparator + strings.Join(segments, SegmentSeparator)
}
