// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/Azure/radius/pkg/radrp/handlers"
	"github.com/Azure/radius/pkg/workloads"
)

// ApplicationModel defines the set of supported component types and related features.
type ApplicationModel interface {
	GetComponents() []ComponentKind
	GetResources() []ResourceType
	LookupComponent(kind string) (ComponentKind, error)
	LookupResource(resourceType string) (ResourceType, error)
}

// ComponentKind represents a component kind supported by the application model.
type ComponentKind interface {
	Kind() string
	Renderer() workloads.WorkloadRenderer
}

// ResourceType represents a type of resource supported by the system.
type ResourceType interface {
	Type() string
	Handler() handlers.ResourceHandler
	HealthHandler() handlers.HealthHandler
}

type applicationModel struct {
	componentlist   []ComponentKind
	componentlookup map[string]ComponentKind
	resourcelist    []ResourceType
	resourcelookup  map[string]ResourceType
}

func (model *applicationModel) GetComponents() []ComponentKind {
	return model.componentlist
}

func (model *applicationModel) LookupComponent(kind string) (ComponentKind, error) {
	component, ok := model.componentlookup[kind]
	if !ok {
		return nil, fmt.Errorf("component kind '%s' is unsupported", kind)
	}

	return component, nil
}

func (model *applicationModel) LookupResource(kind string) (ResourceType, error) {
	resource, ok := model.resourcelookup[kind]
	if !ok {
		return nil, fmt.Errorf("resource type '%s' is unsupported", kind)
	}

	return resource, nil
}

type componentKind struct {
	kind     string
	renderer workloads.WorkloadRenderer
}

func (kind *componentKind) Kind() string {
	return kind.kind
}

func (kind *componentKind) Renderer() workloads.WorkloadRenderer {
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

func (model *applicationModel) GetResources() []ResourceType {
	return model.resourcelist
}

type Handlers struct {
	ResourceHandler handlers.ResourceHandler
	HealthHandler   handlers.HealthHandler
}

func NewModel(renderers map[string]workloads.WorkloadRenderer, handlers map[string]Handlers) ApplicationModel {
	componentlist := []ComponentKind{}
	componentlookup := map[string]ComponentKind{}
	for kind, renderer := range renderers {
		component := componentKind{
			kind:     kind,
			renderer: renderer,
		}

		componentlist = append(componentlist, &component)
		componentlookup[kind] = &component
	}

	resourcelist := []ResourceType{}
	resourcelookup := map[string]ResourceType{}

	// Initialize the resource and health handlers
	for t, handler := range handlers {
		resourceType := resourceType{
			resourceType:  t,
			handler:       handler.ResourceHandler,
			healthHandler: handler.HealthHandler,
		}

		resourcelist = append(resourcelist, &resourceType)
		resourcelookup[t] = &resourceType
	}

	return &applicationModel{
		componentlist:   componentlist,
		componentlookup: componentlookup,
		resourcelist:    resourcelist,
		resourcelookup:  resourcelookup,
	}
}
