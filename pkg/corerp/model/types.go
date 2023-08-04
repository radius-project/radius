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

package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/corerp/handlers"
	"github.com/project-radius/radius/pkg/corerp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

// ApplicationModel defines the set of supported resource types and related features.
type ApplicationModel struct {
	radiusResources      []RadiusResourceModel
	radiusResourceLookup map[string]RadiusResourceModel
	outputResources      []OutputResourceModel
	outputResourceLookup map[resourcemodel.ResourceType]OutputResourceModel
	supportedProviders   map[string]bool
}

// # Function Explanation
//
// GetRadiusResources returns the RadiusResourceModels from the ApplicationModel instance.
func (m ApplicationModel) GetRadiusResources() []RadiusResourceModel {
	return m.radiusResources
}

// # Function Explanation
//
// GetOutputResources returns the OutputResourceModels from the ApplicationModel instance.
func (m ApplicationModel) GetOutputResources() []OutputResourceModel {
	return m.outputResources
}

// LookupRadiusResourceModel is a case insensitive lookup for resourceType
//
// # Function Explanation
//
// LookupRadiusResourceModel does a case-insensitive-lookup for resourceType and returns it,
// or an error if the resource type is unsupported.
func (m ApplicationModel) LookupRadiusResourceModel(resourceType string) (*RadiusResourceModel, error) {
	resource, ok := m.radiusResourceLookup[strings.ToLower(resourceType)]
	if !ok {
		return nil, fmt.Errorf("radius resource type '%s' is unsupported", resourceType)
	}

	return &resource, nil
}

// # Function Explanation
//
// LookupOutputResourceModel looks up an OutputResourceModel from the outputResourceLookup map and returns it,
// or an error if the resourceType is unsupported.
func (m ApplicationModel) LookupOutputResourceModel(resourceType resourcemodel.ResourceType) (*OutputResourceModel, error) {
	resource, ok := m.outputResourceLookup[resourceType]
	if !ok {
		return nil, fmt.Errorf("output resource kind '%s' is unsupported", resourceType)
	}

	return &resource, nil
}

// # Function Explanation
//
// IsProviderSupported checks if the given provider is supported by the application model.
func (m ApplicationModel) IsProviderSupported(provider string) bool {
	return m.supportedProviders[provider]
}

type RadiusResourceModel struct {
	ResourceType string
	Renderer     renderers.Renderer
}

type OutputResourceModel struct {
	ResourceType    resourcemodel.ResourceType
	ResourceHandler handlers.ResourceHandler

	// ResourceTransformer transforms output resource before deploying resource.
	ResourceTransformer func(context.Context, *handlers.PutOptions) error
}

// # Function Explanation
//
// NewModel creates a new ApplicationModel instance by mapping the RadiusResourceModel and OutputResourceModel slices to
// lookup maps and setting the supportedProviders map.
func NewModel(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel, supportedProviders map[string]bool) ApplicationModel {
	radiusResourceLookup := map[string]RadiusResourceModel{}
	for _, radiusResource := range radiusResources {
		radiusResourceLookup[strings.ToLower(radiusResource.ResourceType)] = radiusResource
	}

	outputResourceLookup := map[resourcemodel.ResourceType]OutputResourceModel{}
	for _, outputResource := range outputResources {
		outputResourceLookup[outputResource.ResourceType] = outputResource
	}

	return ApplicationModel{
		radiusResources:      radiusResources,
		radiusResourceLookup: radiusResourceLookup,
		outputResources:      outputResources,
		outputResourceLookup: outputResourceLookup,
		supportedProviders:   supportedProviders,
	}
}
