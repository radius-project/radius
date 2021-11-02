// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/Azure/radius/pkg/handlers"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/pkg/resourcemodel"
)

// ApplicationModel defines the set of supported resource types and related features.
type ApplicationModel interface {
	LookupRenderer(resourceType string) (renderers.Renderer, error)
	LookupHandlers(resourceKind string) (Handlers, error)
	LookupSecretTransformer(transformerName string) (renderers.SecretValueTransformer, error)
	LookupSkipHealthStateCheckResources(identity resourcemodel.ResourceIdentity) bool
}

type applicationModel struct {
	renderersByResourceType      map[string]renderers.Renderer
	handlersByResourceKind       map[string]Handlers
	transformersByName           map[string]renderers.SecretValueTransformer
	skipHealthCheckResourceKinds map[string]bool
}

type Handlers struct {
	ResourceHandler handlers.ResourceHandler
	HealthHandler   handlers.HealthHandler
}

func NewModel(
	renderers map[string]renderers.Renderer,
	handlers map[string]Handlers,
	transformers map[string]renderers.SecretValueTransformer,
	skipHealthCheckResourceKinds map[string]bool) ApplicationModel {
	return &applicationModel{
		renderersByResourceType:      renderers,
		handlersByResourceKind:       handlers,
		transformersByName:           transformers,
		skipHealthCheckResourceKinds: skipHealthCheckResourceKinds,
	}
}

func (model *applicationModel) LookupRenderer(resourceType string) (renderers.Renderer, error) {
	renderer, ok := model.renderersByResourceType[resourceType]
	if !ok {
		return nil, fmt.Errorf("resource kind '%s' is unsupported", resourceType)
	}

	return renderer, nil
}

func (model *applicationModel) LookupHandlers(resourceKind string) (Handlers, error) {
	resourceHandlers, ok := model.handlersByResourceKind[resourceKind]
	if !ok {
		return Handlers{}, fmt.Errorf("resource kind '%s' is unsupported", resourceKind)
	}

	return resourceHandlers, nil
}

func (model *applicationModel) LookupSecretTransformer(transformerName string) (renderers.SecretValueTransformer, error) {
	transformer, ok := model.transformersByName[transformerName]
	if !ok {
		return nil, fmt.Errorf("transformer '%s' is unsupported", transformerName)
	}

	return transformer, nil
}

func (model *applicationModel) LookupSkipHealthStateCheckResources(identity resourcemodel.ResourceIdentity) bool {
	var kind string
	if identity.Kind == resourcekinds.Kubernetes {
		kID := identity.Data.(resourcemodel.KubernetesIdentity)
		kind = kID.Kind
	} else {
		kind = string(identity.Kind)
	}
	_, ok := model.skipHealthCheckResourceKinds[kind]
	return ok
}
