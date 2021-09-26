// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package model

import (
	"fmt"

	"github.com/Azure/radius/pkg/renderers"
)

// ApplicationModelV3 defines the set of supported resource types and related features.
type ApplicationModelV3 interface {
	LookupRenderer(resourceType string) (renderers.Renderer, error)
	LookupHandlers(resourceKind string) (Handlers, error)
	LookupSecretTransformer(transformerName string) (renderers.SecretValueTransformer, error)
}

type applicationModelV3 struct {
	renderersByResourceType map[string]renderers.Renderer
	handlersByResourceKind  map[string]Handlers
	transformersByName      map[string]renderers.SecretValueTransformer
}

func NewModelV3(
	renderers map[string]renderers.Renderer,
	handlers map[string]Handlers,
	transformers map[string]renderers.SecretValueTransformer) ApplicationModelV3 {
	return &applicationModelV3{
		renderersByResourceType: renderers,
		handlersByResourceKind:  handlers,
		transformersByName:      transformers,
	}
}

func (model *applicationModelV3) LookupRenderer(resourceType string) (renderers.Renderer, error) {
	renderer, ok := model.renderersByResourceType[resourceType]
	if !ok {
		return nil, fmt.Errorf("resource kind '%s' is unsupported", resourceType)
	}

	return renderer, nil
}

func (model *applicationModelV3) LookupHandlers(resourceKind string) (Handlers, error) {
	resourceHandlers, ok := model.handlersByResourceKind[resourceKind]
	if !ok {
		return Handlers{}, fmt.Errorf("resource kind '%s' is unsupported", resourceKind)
	}

	return resourceHandlers, nil
}

func (model *applicationModelV3) LookupSecretTransformer(transformerName string) (renderers.SecretValueTransformer, error) {
	transformer, ok := model.transformersByName[transformerName]
	if !ok {
		return nil, fmt.Errorf("transformer '%s' is unsupported", transformerName)
	}

	return transformer, nil
}
