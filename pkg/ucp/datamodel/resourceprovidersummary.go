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

package datamodel

import (
	"fmt"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// ResourceProviderResourceSummaryType is the resource type for the summary of a resource provider.
	//
	// These are **READONLY** virtual resources served from URLs like:
	//
	// /planes/radius/local/providers/
	// /planes/radius/local/providers/Applications.Test
	ResourceProviderSummaryResourceType = "System.Resources/resourceProviderSummaries"
)

// ResourceProviderSummaryIDFromParts returns the resource id for a resource provider summary.
//
// Since ResourceProviderSummary is a virtual resource, the resource id is different from the URL used to access it.
func ResourceProviderSummaryIDFromParts(scope string, name string) (resources.ID, error) {
	return resources.ParseResource(fmt.Sprintf("%s/providers/%s/%s", scope, ResourceProviderSummaryResourceType, name))
}

// ResourceProviderSummary represents the common set of fields for a resource provider.
type ResourceProviderSummary struct {
	v1.BaseResource

	// Properties stores the properties of the resource provider summary.
	Properties ResourceProviderSummaryProperties `json:"properties"`
}

// ResourceTypeName gives the type of the resource.
func (r *ResourceProviderSummary) ResourceTypeName() string {
	return ResourceProviderSummaryResourceType
}

// ResourceProviderSummaryProperties stores the properties of a resource provider summary.
type ResourceProviderSummaryProperties struct {
	// Locations is the list of locations where the resource provider is available.
	Locations map[string]ResourceProviderSummaryPropertiesLocation `json:"locations,omitempty"`

	// ResourceTypes is the list of resource types available in the resource provider.
	ResourceTypes map[string]ResourceProviderSummaryPropertiesResourceType `json:"resourceTypes,omitempty"`
}

// ResourceProviderSummaryLocation represents a location where a resource provider is available.
type ResourceProviderSummaryPropertiesLocation struct {
	// Empty for now.
}

// ResourceProviderSummaryResourceType represents a resource type available in a resource provider.
type ResourceProviderSummaryPropertiesResourceType struct {
	// DefaultAPIVersion is the default API version for the resource type.
	DefaultAPIVersion *string `json:"defaultApiVersion,omitempty"`

	// Capabilities is the list of capabilities supported by the resource type.
	Capabilities []string `json:"capabilities,omitempty"`

	//Description of the resource type.
	Description *string `json:"description,omitempty"`

	// APIVersions is the list of API versions available for the resource type.
	APIVersions map[string]ResourceProviderSummaryPropertiesAPIVersion `json:"apiVersions,omitempty"`
}

// ResourceProviderSummaryAPIVersion represents an API version available in a resource provider.
type ResourceProviderSummaryPropertiesAPIVersion struct {
	// Empty for now.
	Schema map[string]any `json:"schema,omitempty"`
}
