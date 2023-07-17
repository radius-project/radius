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
	id            string
	scopeSegments []ScopeSegment
	typeSegments  []TypeSegment
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

// # Function Explanation
//
// IsEmpty checks if the ID is empty.
func (ri ID) IsEmpty() bool {
	return ri.id == ""
}

// # Function Explanation
//
// IsScope returns true if the ID represents a named scope (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local
func (ri ID) IsScope() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) == 0 && // Not a type
		(len(ri.scopeSegments) == 0 || len(ri.scopeSegments[len(ri.scopeSegments)-1].Name) > 0) // No scope segments or last one is named
}

// # Function Explanation
//
// IsResource returns true if the ID represents a named resource (not a collection or custom action).
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications/my-app
func (ri ID) IsResource() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) > 0 && len(ri.typeSegments[len(ri.typeSegments)-1].Name) > 0 // Has type segments and last one is named
}

// # Function Explanation
//
// IsScopeCollection returns true if the ID represents a collection or custom action on a scope.
//
// Example:
//
//	/planes/radius/local/resourceGroups/resources
func (ri ID) IsScopeCollection() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) == 0 && // No type segments
		len(ri.scopeSegments) > 0 && len(ri.scopeSegments[len(ri.scopeSegments)-1].Name) == 0 // Has scope segments and last one is un-named
}

// # Function Explanation
//
// IsResourceCollection returns true if the ID represents a collection or custom action on a resource.
//
// Example:
//
//	/planes/radius/local/resourceGroups/rg1/providers/Applications.Core/applications
func (ri ID) IsResourceCollection() bool {
	return !ri.IsEmpty() && // Not empty
		len(ri.typeSegments) > 0 && len(ri.typeSegments[len(ri.typeSegments)-1].Name) == 0 // Has type segments and last one is un-named
}

// # Function Explanation
//
// IsUCPQualfied checks if the ID has a prefix of SegmentSeparator and PlanesSegment.
func (ri ID) IsUCPQualfied() bool {
	return strings.HasPrefix(ri.id, SegmentSeparator+PlanesSegment)
}

// # Function Explanation
//
// ScopeSegments gets the slice of root-scope segments.
func (ri ID) ScopeSegments() []ScopeSegment {
	return ri.scopeSegments
}

// # Function Explanation
//
// TypeSegments gets the slice of type segments.
func (ri ID) TypeSegments() []TypeSegment {
	return ri.typeSegments
}

// # Function Explanation
//
// This function returns the "id" field of the given ID instance.
func (ri ID) String() string {
	return ri.id
}

// # Function Explanation
//
// Method FindScope searches through the scopeSegments of the ID instance and returns the Name of the scopeType if found.
func (ri ID) FindScope(scopeType string) string {
	for _, t := range ri.scopeSegments {
		if strings.EqualFold(t.Type, scopeType) {
			return t.Name
		}
	}
	return ""
}

// RootScope returns the root-scope (the part before 'providers'),  taking into account whether the ID is qualified for UCP or not.
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

// # Function Explanation
//
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

// # Function Explanation
//
// ProviderNamespace returns the providers part of the ID
// Examples:
//
//	Applications.Core
func (ri ID) ProviderNamespace() string {
	if len(ri.typeSegments) == 0 {
		return ""
	}
	segments := strings.Split(ri.typeSegments[0].Type, SegmentSeparator)
	return segments[0]
}

// # Function Explanation
//
// IsRadiusRPResource checks if the given ID is a supported Radius resource.
func (ri ID) IsRadiusRPResource() bool {
	return slices.Contains(supportedNamespaces, ri.ProviderNamespace())
}

// # Function Explanation
//
// PlaneNamespace returns the plane part of the UCP ID, or an empty string if the ID is not UCP qualified.
//
// Note: This function does NOT handle invalid IDs. If an invalid ID calls this function then there is
// a chance that it is going to trigger a panic.
//
// Examples:
//
//	radius
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

// # Function Explanation
//
// RoutingScope returns the routing-scope (the part after 'providers') - it is composed of the type and name segments of the ID instance.
//
// Examples:
//
//	/Applications.Core/applications/my-app
func (ri ID) RoutingScope() string {
	segments := []string{}
	for _, t := range ri.typeSegments {
		segments = append(segments, t.Type)
		if t.Name != "" {
			segments = append(segments, t.Name)
		}
	}

	return strings.Join(segments, SegmentSeparator)
}

// # Function Explanation
//
// Type returns the fully-qualified resource type of a ResourceID, or an empty string if the type cannot be determined.
func (ri ID) Type() string {
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

// # Function Explanation
//
// QualifiedName gets the fully-qualified resource name (eg. `radiusv3/myapp/mycontainer`) by joining the type segments with the SegmentSeparator.
func (ri ID) QualifiedName() string {
	names := make([]string, len(ri.typeSegments))
	for i, t := range ri.typeSegments {
		names[i] = t.Name
	}
	return strings.Join(names, SegmentSeparator)
}

// # Function Explanation
//
// Name gets the resource or scope name.
func (ri ID) Name() string {
	if len(ri.typeSegments) == 0 && len(ri.scopeSegments) == 0 {
		return ""
	}

	if len(ri.typeSegments) == 0 {
		return ri.scopeSegments[len(ri.scopeSegments)-1].Name
	}

	return ri.typeSegments[len(ri.typeSegments)-1].Name
}

// # Function Explanation
//
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

// # Function Explanation
//
// Append appends a resource type segment to the ID and returns the resulting ID. If the ID is UCP qualified, it will
// return a UCP qualified ID, otherwise it will return a relative ID.
func (ri ID) Append(resourceType TypeSegment) ID {
	types := append(ri.typeSegments, resourceType)

	if ri.IsUCPQualfied() {
		result, err := Parse(MakeUCPID(ri.scopeSegments, types...))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	} else {
		result, err := Parse(MakeRelativeID(ri.scopeSegments, types...))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	}
}

// # Function Explanation
//
// Truncate removes the last type/name pair for a resource id or scope id. Calling truncate on a top level resource or scope has no effect.
func (ri ID) Truncate() ID {
	if len(ri.typeSegments) == 0 && len(ri.scopeSegments) == 0 {
		return ri // Top level scope already
	}

	if len(ri.typeSegments) > 0 && len(ri.typeSegments) < 2 {
		return ri // Top level resource already
	}

	if len(ri.typeSegments) == 0 {
		// Truncate the root scope
		if ri.IsUCPQualfied() {
			result, err := Parse(MakeUCPID(ri.scopeSegments[0:len(ri.scopeSegments)-1], []TypeSegment{}...))
			if err != nil {
				panic(err) // Should not be possible.
			}

			return result
		} else {
			result, err := Parse(MakeRelativeID(ri.scopeSegments[0:len(ri.scopeSegments)-1], []TypeSegment{}...))
			if err != nil {
				panic(err) // Should not be possible.
			}

			return result
		}
	}

	// Truncate the resource type
	if ri.IsUCPQualfied() {
		result, err := Parse(MakeUCPID(ri.scopeSegments, ri.typeSegments[0:len(ri.typeSegments)-1]...))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	} else {
		result, err := Parse(MakeRelativeID(ri.scopeSegments, ri.typeSegments[0:len(ri.typeSegments)-1]...))
		if err != nil {
			panic(err) // Should not be possible.
		}

		return result
	}
}

// # Function Explanation
//
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

// # Function Explanation
//
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

// # Function Explanation
//
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

// # Function Explanation
//
// Parse parses a resource ID. Parse will parse ALL valid resource IDs in the most permissive way.
// Most code should use a more specific function like ParseResource to parse the specific kind of ID
// they want to handle.
func Parse(id string) (ID, error) {
	isUCPQualified := false
	if strings.HasPrefix(id, SegmentSeparator+PlanesSegment) {
		isUCPQualified = true
		id = strings.TrimPrefix(id, SegmentSeparator+PlanesSegment)

		// Handles /planes and /planes/
		if id == "" || id == "/" {
			normalized := MakeUCPID([]ScopeSegment{}, []TypeSegment{}...)
			return ID{
				id:            normalized,
				scopeSegments: []ScopeSegment{},
				typeSegments:  []TypeSegment{},
			}, nil
		}
	}

	if id == "/" {
		normalized := MakeRelativeID([]ScopeSegment{}, []TypeSegment{}...)
		return ID{
			id:            normalized,
			scopeSegments: []ScopeSegment{},
			typeSegments:  []TypeSegment{},
		}, nil
	}

	// If UCP forwards a request to the RP, the incoming URL
	// will not have the UCP Prefix but will have a planes segment
	isUCPForwarded := false
	if strings.HasPrefix(id, SegmentSeparator+PlanesSegment) {
		isUCPForwarded = true
		id = strings.TrimPrefix(id, SegmentSeparator+PlanesSegment)
	}

	// trim the leading and ending / so we don't end up with an empty segment - we disallow
	// empty segments in the middle of the string
	id = strings.TrimPrefix(id, SegmentSeparator)
	id = strings.TrimSuffix(id, SegmentSeparator)

	// The minimum segment count is 2 since we can parse "root scope only" ids.
	segments := strings.Split(id, SegmentSeparator)

	if len(segments) < 2 {
		return ID{}, invalid(id)
	}

	// Check up front for empty segments
	for _, s := range segments {
		if s == "" {
			return ID{}, invalid(id)
		}
	}

	// Parse scopes - iterate until we get to "providers"
	//
	// Each id has a 'scope' portion and an optional 'resource'. The 'providers' segment is the
	// delimiter between these.
	scopes := []ScopeSegment{}

	i := 0
	for i < len(segments) {
		// We're done parsing scopes
		if strings.ToLower(segments[i]) == ProvidersSegment {
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
			return ID{}, invalid(id)
		}

		if isUCPForwarded && i == 0 {
			// Add the planes segment to the scope
			segments[i] = PlanesSegment + SegmentSeparator + segments[i]
		}
		scopes = append(scopes, ScopeSegment{Type: segments[i], Name: segments[i+1]})
		i += 2
	}

	// We might not have a "providers" segment at all, if that's the case then this ID refers to
	// a scope.
	if len(segments)-i == 0 {
		normalized := ""
		if isUCPQualified {
			normalized = MakeUCPID(scopes, []TypeSegment{}...)
		} else {
			normalized = MakeRelativeID(scopes, []TypeSegment{}...)
		}

		return ID{
			id:            normalized,
			scopeSegments: scopes,
			typeSegments:  []TypeSegment{},
		}, nil
	}

	// Now that're past providers, we're looking for the namespace/type - that is
	// at least 2 segments.
	if len(segments)-i < 2 {
		return ID{}, invalid(id)
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

	normalized := ""
	if isUCPQualified {
		normalized = MakeUCPID(scopes, types...)
	} else {
		normalized = MakeRelativeID(scopes, types...)
	}

	return ID{
		id:            normalized,
		scopeSegments: scopes,
		typeSegments:  types,
	}, nil
}

func invalid(id string) error {
	return fmt.Errorf("'%s' is not a valid resource id", id)
}

// # Function Explanation
//
// MakeUCPID creates a fully-qualified UCP resource ID, from the given scopes and resource types.
func MakeUCPID(scopes []ScopeSegment, resourceTypes ...TypeSegment) string {
	segments := []string{
		PlanesSegment,
	}
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

	return SegmentSeparator + strings.Join(segments, SegmentSeparator)
}

// # Function Explanation
//
// MakeRelativeID makes a plane-relative resource ID (ARM style) from a slice of ScopeSegment and a variadic of TypeSegment..
func MakeRelativeID(scopes []ScopeSegment, resourceTypes ...TypeSegment) string {
	segments := []string{}
	for _, scope := range scopes {
		segments = append(segments, scope.Type, scope.Name)
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

	return SegmentSeparator + strings.Join(segments, SegmentSeparator)
}
