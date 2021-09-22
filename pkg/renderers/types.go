// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/outputresource"
)

type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource RendererResource) ([]azresources.ResourceID, error)
	Render(ctx context.Context, resource RendererResource, dependencies map[string]RendererDependency) (RendererOutput, error)
}

type RendererResource struct {
	ApplicationName string
	ResourceName    string
	ResourceType    string
	Definition      map[string]interface{}
}

type RendererDependency struct {
	ResourceID     azresources.ResourceID
	Definition     map[string]interface{}
	ComputedValues map[string]interface{}
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]ComputedValueReference
	SecretValues   map[string]SecretValueReference
}

type ComputedValueReference struct {
	LocalID       string
	ValueSelector string
}

type SecretValueReference struct {
	LocalID       string
	Action        string
	ValueSelector string
}
