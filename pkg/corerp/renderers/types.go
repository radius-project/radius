// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package renderers

import (
	"context"

	"github.com/project-radius/radius/pkg/armrpc/api/conv"
	"github.com/project-radius/radius/pkg/corerp/datamodel"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/rp"
	"github.com/project-radius/radius/pkg/rp/outputresource"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/corerp/renderers Renderer
type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error)
	Render(ctx context.Context, resource conv.DataModelInterface, options RenderOptions) (RendererOutput, error)
}

type RenderOptions struct {
	Dependencies map[string]RendererDependency
	Environment  EnvironmentOptions
}

// Represents a dependency of the resource currently being rendered. Currently dependencies are always Radius resources.
type RendererDependency struct {
	// ResourceID is the resource ID of the Radius resource that is the dependency.
	ResourceID resources.ID

	// Definition is the definition (`properties` node) of the dependency.
	Definition map[string]interface{}

	// Resource is the datamodel of depedency resource.
	Resource conv.DataModelInterface

	// ComputedValues is a map of the computed values and secrets of the dependency.
	ComputedValues map[string]interface{}

	// OutputResources is a map of the output resource identities of the dependency. The map is keyed on the LocalID of the output resource.
	OutputResources map[string]resourcemodel.ResourceIdentity
}

// EnvironmentOptions represents the options for the linked environment resource.
type EnvironmentOptions struct {
	// Namespace represents the Kubernetes namespace.
	Namespace string
	// Providers represents the cloud provider's configurations.
	CloudProviders *datamodel.Providers
	// Gateway represents the gateway options.
	Gateway GatewayOptions
	// Identity represents identity of the environment.
	Identity *rp.IdentitySettings
}

type GatewayOptions struct {
	PublicEndpointOverride bool
	Hostname               string
	Port                   string
	ExternalIP             string
}

type RendererOutput struct {
	Resources      []outputresource.OutputResource
	ComputedValues map[string]rp.ComputedValueReference
	SecretValues   map[string]rp.SecretValueReference

	// RadiusResource is the original Radius resource model.
	RadiusResource conv.DataModelInterface
}
