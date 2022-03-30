// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/project-radius/radius/pkg/handlers"
	"github.com/project-radius/radius/pkg/renderers"
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

func (m ApplicationModel) GetRadiusResources() []RadiusResourceModel {
	return m.radiusResources
}

func (m ApplicationModel) GetOutputResources() []OutputResourceModel {
	return m.outputResources
}

func (m ApplicationModel) LookupRadiusResourceModel(resourceType string) (*RadiusResourceModel, error) {
	resource, ok := m.radiusResourceLookup[resourceType]
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
	HealthHandler          handlers.HealthHandler
	ResourceHandler        handlers.ResourceHandler
	SecretValueTransformer renderers.SecretValueTransformer

	// ShouldSupportHealthMonitorFunc is a function that executes per resource identity to determine whether
	// the resource should be monitored for health reporting. Health monitoring is OPT-IN.
	ShouldSupportHealthMonitorFunc func(resourceType resourcemodel.ResourceType) bool
}

func (or OutputResourceModel) SupportsHealthMonitor(resourceType resourcemodel.ResourceType) bool {
	if or.ShouldSupportHealthMonitorFunc == nil {
		return false
	}

	return or.ShouldSupportHealthMonitorFunc(resourceType)
}

func NewModel(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel, supportedProviders map[string]bool) ApplicationModel {
	radiusResourceLookup := map[string]RadiusResourceModel{}
	for _, radiusResource := range radiusResources {
		radiusResourceLookup[radiusResource.ResourceType] = radiusResource
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
