// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
)

type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []azresources.ResourceID, azureResourceIDs []azresources.ResourceID, err error)
	Render(ctx context.Context, resource conv.DataModelInterface, options RenderOptions) (RendererOutput, error)
}

type RenderOptions struct {
	Dependencies map[string]RendererDependency
	Runtime      RuntimeOptions
}

// Represents a dependency of the resource currently being rendered. Currently dependencies are always Radius resources.
type RendererDependency struct {
	// ResourceID is the resource ID of the Radius resource that is the dependency.
	ResourceID azresources.ResourceID

	// Definition is the definition (`properties` node) of the dependency.
	Definition map[string]interface{}

	// ComputedValues is a map of the computed values and secrets of the dependency.
	ComputedValues map[string]interface{}

	// OutputResources is a map of the output resource identities of the dependency. The map is keyed on the LocalID of the output resource.
	OutputResources map[string]resourcemodel.ResourceIdentity
}

type RuntimeOptions struct {
	Gateway GatewayOptions
}

type GatewayOptions struct {
	PublicEndpointOverride bool
	PublicIP               string
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]ComputedValueReference
}

// ComputedValueReference represents a non-secret value that can accessed once the output resources
// have been deployed.
type ComputedValueReference struct {
	// ComputedValueReference might hold a static value in `.Value` or might be a reference
	// that needs to be looked up.
	//
	// If `.Value` is set then treat this as a static value.
	//
	// If `.Value == nil` then use the `.PropertyReference` or to look up a property in the property
	// bag returned from deploying the resource via `handler.Put`.
	//
	// If `.Value == nil` && `.PropertyReference` is unset, then use JSONPointer to evaluate a JSON path
	// into the 'resource'.

	// LocalID specifies the output resource to be used for lookup. Does not apply with `.Value`
	LocalID string

	// Value specifies a static value to copy to computed values.
	Value interface{}

	// PropertyReference specifies a property key to look up in the resource's *persisted properties*.
	PropertyReference string

	// JSONPointer specifies a JSON Pointer that cn be used to look up the value in the resource's body.
	JSONPointer string
}
