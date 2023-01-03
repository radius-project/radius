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

const (
	// DefaultPort represents the default port of HTTP endpoint.
	DefaultPort int32 = 80

	// DefaultSecurePort represents the default port of HTTPS endpoint.
	DefaultSecurePort int32 = 443
)

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/project-radius/radius/pkg/corerp/renderers Renderer
type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource conv.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error)
	Render(ctx context.Context, resource conv.DataModelInterface, options RenderOptions) (RendererOutput, error)
}

type RenderOptions struct {
	Dependencies map[string]RendererDependency
	Environment  EnvironmentOptions
	Application  ApplicationOptions
}

// Represents a dependency of the resource currently being rendered. Currently dependencies are always Radius resources.
type RendererDependency struct {
	// ResourceID is the resource ID of the Radius resource that is the dependency.
	ResourceID resources.ID

	// Resource is the datamodel of depedency resource.
	Resource conv.DataModelInterface

	// ComputedValues is a map of the computed values and secrets of the dependency.
	ComputedValues map[string]any

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
	// KubernetesMetadata represents the Environment KubernetesMetadata extension.
	KubernetesMetadata *datamodel.KubeMetadataExtension
}

// ApplicationOptions represents the options for the linked application resource.
type ApplicationOptions struct {
	// KubernetesMetadata represents the Application KubernetesMetadata extension.
	KubernetesMetadata *datamodel.KubeMetadataExtension
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
