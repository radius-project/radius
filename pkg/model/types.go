// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcemodel"
)

// ApplicationModel defines the set of supported resource types and related features.
type ApplicationModel struct {
	radiusResources      []RadiusResourceModel
	radiusResourceLookup map[string]RadiusResourceModel
	outputResources      []OutputResourceModel
	outputResourceLookup map[string]OutputResourceModel
}

func (m ApplicationModel) GetRadiusResources() []RadiusResourceModel {
	return m.radiusResources
}

func (m ApplicationModel) GetOutputResources() []OutputResourceModel {
	return m.outputResources
}

func (m ApplicationModel) LookupRadiusResource(resourceType string) (*RadiusResourceModel, error) {
	resource, ok := m.radiusResourceLookup[resourceType]
	if !ok {
		return nil, fmt.Errorf("radius resource type '%s' is unsupported", resourceType)
	}

	return &resource, nil
}

func (m ApplicationModel) LookupOutputResource(resourceKind string) (*OutputResourceModel, error) {
	resource, ok := m.outputResourceLookup[resourceKind]
	if !ok {
		return nil, fmt.Errorf("output resource kind '%s' is unsupported", resourceKind)
	}

	return &resource, nil
}

type RadiusResourceModel struct {
	ResourceType string
	Renderer     renderers.Renderer
}

type OutputResourceModel struct {
	Kind                   string
	HealthHandler          handlers.HealthHandler
	ResourceHandler        handlers.ResourceHandler
	SecretValueTransformer renderers.SecretValueTransformer

	// ShouldSupportHealthMonitorFunc is a function that executes per resource identity to determine whether
	// the resource should be monitored for health reporting. Health monitoring is OPT-IN.
	ShouldSupportHealthMonitorFunc func(identity resourcemodel.ResourceIdentity) bool
}

func (or OutputResourceModel) SupportsHealthMonitor(identity resourcemodel.ResourceIdentity) bool {
	if or.ShouldSupportHealthMonitorFunc == nil {
		return false
	}

	return or.ShouldSupportHealthMonitorFunc(identity)
}

func NewModel(radiusResources []RadiusResourceModel, outputResources []OutputResourceModel) ApplicationModel {
	radiusResourceLookup := map[string]RadiusResourceModel{}
	for _, radiusResource := range radiusResources {
		radiusResourceLookup[radiusResource.ResourceType] = radiusResource
	}

	outputResourceLookup := map[string]OutputResourceModel{}
	for _, outputResource := range outputResources {
		outputResourceLookup[outputResource.Kind] = outputResource
	}

	return ApplicationModel{
		radiusResources:      radiusResources,
		radiusResourceLookup: radiusResourceLookup,
		outputResources:      outputResources,
		outputResourceLookup: outputResourceLookup,
	}
}
