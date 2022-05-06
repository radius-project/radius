// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"fmt"
	"strings"
)

const (
	SegmentSeparator = "/"
	PlanesSegment    = "planes"
	ProvidersSegment = "providers"
	UCPPrefix        = "ucp:"
)

// ID represents an ARM or UCP resource id. ID is immutable once created. Use Parse() to create IDs and use
// String() to convert back to strings.
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
	Name string
}

// IsEmpty returns true if the ID is empty.
func (ri ID) IsEmpty() bool {
	return ri.id == ""
}

// IsCollection returns true if the ID represents a collection (final segment has no name).
func (ri ID) IsCollection() bool {
	if ri.IsScope() {
		return ri.scopeSegments[len(ri.scopeSegments)-1].Name == ""
	}

	return ri.typeSegments[len(ri.typeSegments)-1].Name == ""
}

// IsScope returns true if the ID represents a scope.
func (ri ID) IsScope() bool {
	return !ri.IsEmpty() && len(ri.typeSegments) == 0
}

// IsUCPQualfied returns true if the ID has a UCP qualifier ('ucp:/').
func (ri ID) IsUCPQualfied() bool {
	return strings.HasPrefix(ri.id, UCPPrefix)
}

// ScopeSegments gets the slice of root-scope segments.
func (ri ID) ScopeSegments() []ScopeSegment {
	return ri.scopeSegments
}

// TypeSegments gets the slice of type segments.
func (ri ID) TypeSegments() []TypeSegment {
	return ri.typeSegments
}

func (ri ID) String() string {
	return ri.id
}

// RootScope returns the root-scope (the part before 'providers'). This includes 'ucp:' prefix.
//
// Examples:
//	/subscriptions{guid}/resourceGroups/cool-group
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
		return UCPPrefix + SegmentSeparator + PlanesSegment + SegmentSeparator + joined
	}

	return SegmentSeparator + joined
}

// RoutingScope returns the routing-scope (the part after 'providers').
//
// Examples:
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

// Type returns the fully-qualified resource type of a ResourceID.
func (ri ID) Type() string {
	types := make([]string, len(ri.typeSegments))
	for i, t := range ri.typeSegments {
		types[i] = t.Type
	}
	return strings.Join(types, SegmentSeparator)
}

// QualifiedName gets the fully-qualified resource name (eg. `radiusv3/myapp/mycontainer`).
func (ri ID) QualifiedName() string {
	names := make([]string, len(ri.typeSegments))
	for i, t := range ri.typeSegments {
		names[i] = t.Name
	}
	return strings.Join(names, SegmentSeparator)
}

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

// Append appends a type/name pair to the ResourceID.
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

// Truncate removes the last type/name pair of the ResourceID. Calling truncate on a top level resource has no effect.
func (ri ID) Truncate() ID {
	if len(ri.typeSegments) < 2 {
		return ri
	}

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

// Parse parses a resource ID.
func Parse(id string) (ID, error) {
	isUCPQualified := false
	if strings.HasPrefix(id, UCPPrefix+SegmentSeparator+PlanesSegment) {
		isUCPQualified = true
		id = strings.TrimPrefix(id, UCPPrefix+SegmentSeparator+PlanesSegment)
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
	scopes := []ScopeSegment{}
	i := 0
	for i < len(segments) {
		if len(segments)-i < 2 {
			// odd number of non-providers segments remaining, this is invalid.
			return ID{}, invalid(id)
		}

		// We're done parsing scopes
		if strings.ToLower(segments[i]) == ProvidersSegment {
			i++ // advance past "providers"
			break
		}

		if strings.ToLower(segments[i+1]) == ProvidersSegment {
			// odd number of non-providers segments inside the root scope, this is invalid.
			return ID{}, invalid(id)
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

// MakeUCPID creates a fully-qualified UCP resource ID.
func MakeUCPID(scopes []ScopeSegment, resourceTypes ...TypeSegment) string {
	segments := []string{
		PlanesSegment,
	}
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

	return UCPPrefix + SegmentSeparator + strings.Join(segments, SegmentSeparator)
}

// MakeRelativeID makes a plane-relative resource ID (ARM style).
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
