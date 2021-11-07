// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/renderers"
)

// ApplicationModel defines the set of supported resource types and related features.
type ApplicationModel interface {
	GetResources() []ResourceKind
	GetOutputResources() []OutputResourceType
	LookupResource(kind string) (ResourceKind, error)
	LookupOutputResource(resourceType string) (OutputResourceType, error)
	GetSecretValueTransformer(name string) (renderers.SecretValueTransformer, error)
}

// ResourceKind represents a resource kind supported by the application model.
type ResourceKind interface {
	Kind() string
	Renderer() renderers.Renderer
}

// ResourceType represents a type of resource supported by the system.
type OutputResourceType interface {
	Type() string
	Handler() handlers.ResourceHandler
	HealthHandler() handlers.HealthHandler
}

type applicationModel struct {
	resourceList           []ResourceKind
	resourceLookup         map[string]ResourceKind
	outputResourceList     []OutputResourceType
	outputResourceLookup   map[string]OutputResourceType
	secretValueTranformers map[string]renderers.SecretValueTransformer
}

func (model *applicationModel) GetResources() []ResourceKind {
	return model.resourceList
}

func (model *applicationModel) LookupResource(kind string) (ResourceKind, error) {
	resource, ok := model.resourceLookup[kind]
	if !ok {
		return nil, fmt.Errorf("resource kind '%s' is unsupported", kind)
	}

	return resource, nil
}

func (model *applicationModel) LookupOutputResource(kind string) (OutputResourceType, error) {
	resource, ok := model.outputResourceLookup[kind]
	if !ok {
		return nil, fmt.Errorf("resource type '%s' is unsupported", kind)
	}

	return resource, nil
}

type resourceKind struct {
	kind     string
	renderer renderers.Renderer
}

func (kind *resourceKind) Kind() string {
	return kind.kind
}

func (kind *resourceKind) Renderer() renderers.Renderer {
	return kind.renderer
}

type resourceType struct {
	resourceType  string
	handler       handlers.ResourceHandler
	healthHandler handlers.HealthHandler
}

func (rt *resourceType) Type() string {
	return rt.resourceType
}

func (rt *resourceType) Handler() handlers.ResourceHandler {
	return rt.handler
}

func (rt *resourceType) HealthHandler() handlers.HealthHandler {
	return rt.healthHandler
}

func (model *applicationModel) GetOutputResources() []OutputResourceType {
	return model.outputResourceList
}

func (model *applicationModel) GetSecretValueTransformer(name string) (renderers.SecretValueTransformer, error) {
	transformer, ok := model.secretValueTranformers[name]
	if !ok {
		return nil, fmt.Errorf("transformer %q not found", name)
	}

	return transformer, nil
}

type Handlers struct {
	ResourceHandler handlers.ResourceHandler
	HealthHandler   handlers.HealthHandler
}

func NewModel(renderers map[string]renderers.Renderer, handlers map[string]Handlers, transformers map[string]renderers.SecretValueTransformer) ApplicationModel {
	resourceList := []ResourceKind{}
	resourceLookup := map[string]ResourceKind{}
	for kind, renderer := range renderers {
		resource := resourceKind{
			kind:     kind,
			renderer: renderer,
		}

		resourceList = append(resourceList, &resource)
		resourceLookup[kind] = &resource
	}

	outputResourceList := []OutputResourceType{}
	outputResourceLookup := map[string]OutputResourceType{}

	// Initialize the resource and health handlers
	for t, handler := range handlers {
		resourceType := resourceType{
			resourceType:  t,
			handler:       handler.ResourceHandler,
			healthHandler: handler.HealthHandler,
		}

		outputResourceList = append(outputResourceList, &resourceType)
		outputResourceLookup[t] = &resourceType
	}

	return &applicationModel{
		resourceList:           resourceList,
		resourceLookup:         resourceLookup,
		outputResourceList:     outputResourceList,
		outputResourceLookup:   outputResourceLookup,
		secretValueTranformers: transformers,
	}
}
