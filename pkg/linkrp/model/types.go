// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/linkrp/handlers"
	"github.com/project-radius/radius/pkg/linkrp/renderers"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
)

// ApplicationModel defines the set of supported resource types and related features.
type ApplicationModel struct {
	radiusResources      []RadiusResourceModel
	radiusResourceLookup map[string]RadiusResourceModel
	outputResources      []OutputResourceModel
	outputResourceLookup map[resourcemodel.ResourceType]OutputResourceModel
	supportedProviders   map[string]bool
	recipeModel          RecipeModel
}

func (m ApplicationModel) GetRadiusResources() []RadiusResourceModel {
	return m.radiusResources
}

func (m ApplicationModel) GetOutputResources() []OutputResourceModel {
	return m.outputResources
}

func (m ApplicationModel) GetRecipeModel() RecipeModel {
	return m.recipeModel
}

// LookupRadiusResourceModel is a case insensitive lookup for resourceType
func (m ApplicationModel) LookupRadiusResourceModel(resourceType string) (*RadiusResourceModel, error) {
	resource, ok := m.radiusResourceLookup[strings.ToLower(resourceType)]
	if !ok {
		return nil, fmt.Errorf("radius resource type '%s' is unsupported", resourceType)
	}

	return &resource, nil
}

func (m ApplicationModel) LookupOutputResourceModel(resourceType resourcemodel.ResourceType) (*OutputResourceModel, error) {
	resource, ok := m.outputResourceLookup[resourceType]
	if !ok {
		return nil, fmt.Errorf("output resource kind '%s' is unsupported", resourceType)
	}

	return &resource, nil
}

func (m ApplicationModel) IsProviderSupported(provider string) bool {
	return m.supportedProviders[provider]
}

type RadiusResourceModel struct {
	ResourceType string
	Renderer     renderers.Renderer
}

type OutputResourceModel struct {
	ResourceType           resourcemodel.ResourceType
	ResourceHandler        handlers.ResourceHandler
	SecretValueTransformer rp.SecretValueTransformer
}

type RecipeModel struct {
	RecipeHandler handlers.RecipeHandler
}

func NewModel(recipeModel RecipeModel, radiusResources []RadiusResourceModel, outputResources []OutputResourceModel, supportedProviders map[string]bool) ApplicationModel {
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
		recipeModel:          recipeModel,
	}
}
