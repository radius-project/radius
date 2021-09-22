// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"
	"encoding/json"
	"fmt"

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
	ResourceID      azresources.ResourceID
	Definition      map[string]interface{}
}

type RendererDependency struct {
	ResourceID     azresources.ResourceID
	Definition     map[string]interface{}
	ComputedValues map[string]ComputedValueReference
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]ComputedValueReference
	SecretValues   map[string]SecretValueReference
}

// ComputedValueReference represents a non-secret value that can accessed once the output resources
// have been deployed.
type ComputedValueReference struct {
	// ComputedValueReference might hold a static value in `.Value` or might be a reference
	// that needs to be looked up. If `.Value` is set then treat this as a static value.
	// If `.Value == nil` then use the `.PropertyReference` to look up a property in the property
	// bag returned from deploying the resource via `handler.Put`.

	LocalID           string
	PropertyReference string
	Value             interface{}
}

// SecretValueReference represents a secret value that can accessed on the output resources
// have been deployed.
type SecretValueReference struct {
	// SecretValueReference always needs to be resolved against a deployed resource. These
	// are secrets so we don't want to store them.

	LocalID       string
	Action        string
	ValueSelector string
}

// ConvertDefinition can be used to convert `.Definition` to a strongly-typed struct.
func (r RendererResource) ConvertDefinition(properties interface{}) error {
	b, err := json.Marshal(r.Definition)
	if err != nil {
		return fmt.Errorf("failed to marshal resource definition: %w", err)
	}

	err = json.Unmarshal(b, properties)
	if err != nil {
		return fmt.Errorf("failed to unmarshal resource definition: %w", err)
	}

	return nil
}
