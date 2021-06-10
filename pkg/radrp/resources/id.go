// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources

import (
	"fmt"
	"strings"
)

// ResourceID represents an Azure resource id.
type ResourceID struct {
	ID             string
	SubscriptionID string
	ResourceGroup  string
	Types          []ResourceType
}

// ResourceType represents one of the type/name pairs of a resource ID.
type ResourceType struct {
	Type string
	Name string
}

type KnownType struct {
	Types []ResourceType
}

// Type prints the fully-qualified resource type of a KnownType.
func (t KnownType) Type() string {
	types := make([]string, len(t.Types))
	for i, t := range t.Types {
		types[i] = t.Type
	}
	return strings.Join(types, "/")
}

// Kind returns the fully-qualified resource kind of a ResourceID.
func (ri ResourceID) Kind() string {
	types := make([]string, len(ri.Types))
	for i, t := range ri.Types {
		types[i] = t.Type
	}
	return strings.Join(types, "/")
}

// Name gets the resource name.
func (ri ResourceID) Name() string {
	if len(ri.Types) == 0 {
		return ""
	}

	return ri.Types[len(ri.Types)-1].Name
}

// Format implements formating for use in logging.
func (ri ResourceID) Format(f fmt.State, c rune) {
	// This is a Radius-specific opinion since it's just for our logging.
	if len(ri.Types) == 0 {
		_, _ = f.Write([]byte(fmt.Sprintf("{ResourceGroup: %v}", ri.ResourceGroup)))
		return
	}

	last := ri.Types[len(ri.Types)-1]
	if last.Name == "" {
		_, _ = f.Write([]byte(fmt.Sprintf("{collection: %v}", last.Type)))
		return
	}

	_, _ = f.Write([]byte(fmt.Sprintf("{%v: %v}", last.Type, last.Name)))
}

// ValidateResourceType validates that the resource ID matches the expected type.
func (ri ResourceID) ValidateResourceType(t KnownType) error {

	if len(ri.Types) != len(t.Types) {
		return invalidType(ri.ID)
	}

	for i, rt := range t.Types {
		// Mismatched type
		if !strings.EqualFold(rt.Type, ri.Types[i].Type) {
			return invalidType(ri.ID)
		}

		// A collection was expected and this has a name.
		if rt.Name == "" && ri.Types[i].Name != "" {
			return invalidType(ri.ID)
		}

		// A resource was expected and this a collection.
		if rt.Name != "" && ri.Types[i].Name == "" {
			return invalidType(ri.ID)
		}
	}

	return nil
}

func invalidType(id string) error {
	return fmt.Errorf("resource '%v' does not match the expected resource type", id)
}

// Parse parses a resource ID.
func Parse(id string) (ResourceID, error) {
	// trim the leading and ending / so we don't end up with an empty segment - we disallow
	// empty segments in the middle of the string
	id = strings.TrimPrefix(id, "/")
	id = strings.TrimSuffix(id, "/")

	// The minimum segment count is 7 since top level resources always have a namespace segment and a
	// type segment.
	// Ex: /subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders
	segments := strings.Split(id, "/")
	if len(segments) < 7 {
		return ResourceID{}, invalid(id)
	}

	// Check up front for empty segments
	for _, s := range segments {
		if s == "" {
			return ResourceID{}, invalid(id)
		}
	}

	if strings.ToLower(segments[0]) != "subscriptions" {
		return ResourceID{}, invalid(id)
	}

	subscriptionID := segments[1]

	if strings.ToLower(segments[2]) != "resourcegroups" {
		return ResourceID{}, invalid(id)
	}

	resourceGroup := segments[3]

	if strings.ToLower(segments[4]) != "providers" {
		return ResourceID{}, invalid(id)
	}

	rt := ResourceType{Type: fmt.Sprintf("%s/%s", segments[5], segments[6])}
	if len(segments) > 7 {
		rt.Name = segments[7]
	}

	types := []ResourceType{rt}

	i := 8
	for {
		if i >= len(segments) {
			break
		}

		rt := ResourceType{Type: segments[i]}
		i = i + 1

		// check for a resource name
		if i >= len(segments) {
			// This is a collection.
			types = append(types, rt)
			break
		}

		// we have a name - keep parsing
		rt.Name = segments[i]
		i = i + 1
		types = append(types, rt)
	}

	return ResourceID{
		ID:             MakeID(subscriptionID, resourceGroup, types[0], types[1:]...),
		SubscriptionID: subscriptionID,
		ResourceGroup:  resourceGroup,
		Types:          types,
	}, nil
}

func invalid(id string) error {
	return fmt.Errorf("'%s' is not a valid resource id", id)
}

// MakeID creates a fully-qualified resource ID.
func MakeID(subscriptionID string, resourceGroup string, resourceType ResourceType, resourceTypes ...ResourceType) string {
	segments := []string{
		"subscriptions",
		subscriptionID,
		"resourceGroups",
		resourceGroup,
		"providers",
	}

	segments = append(segments, resourceType.Type)
	if resourceType.Name != "" {
		segments = append(segments, resourceType.Name)
	}

	for _, rt := range resourceTypes {
		segments = append(segments, rt.Type)
		if rt.Name != "" {
			segments = append(segments, rt.Name)
		}
	}

	return "/" + strings.Join(segments, "/")
}

// MakeCollectionURITemplate creates a URI template for a collection given the provided resource types.
func MakeCollectionURITemplate(t KnownType) string {
	segments := []string{
		"subscriptions",
		fmt.Sprintf("{%s}", SubscriptionIDKey),
		"resourceGroups",
		fmt.Sprintf("{%s}", ResourceGroupKey),
		"providers",
	}

	if len(t.Types) == 0 {
		return "/" + strings.Join(segments, "/")
	}

	segments = append(segments, t.Types[0].Type)

	for i, rt := range t.Types[1:] {
		segments = append(segments, fmt.Sprintf("{resourceName%d}", i))
		segments = append(segments, rt.Type)
	}

	return "/" + strings.Join(segments, "/")
}

// MakeResourceURITemplate creates a URI template for a resource given the provided resource types.
func MakeResourceURITemplate(t KnownType) string {
	segments := []string{
		"subscriptions",
		fmt.Sprintf("{%s}", SubscriptionIDKey),
		"resourceGroups",
		fmt.Sprintf("{%s}", ResourceGroupKey),
		"providers",
	}

	if len(t.Types) == 0 {
		return "/" + strings.Join(segments, "/")
	}

	segments = append(segments, t.Types[0].Type)
	segments = append(segments, "{resourceName0}")

	for i, rt := range t.Types[1:] {
		segments = append(segments, rt.Type)
		segments = append(segments, fmt.Sprintf("{resourceName%d}", i+1))
	}

	return "/" + strings.Join(segments, "/")
}
