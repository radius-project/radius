/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package renderers

import (
	"context"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// DefaultPort represents the default port of HTTP endpoint.
	DefaultPort int32 = 80

	// DefaultSecurePort represents the default port of HTTPS endpoint.
	DefaultSecurePort int32 = 443
)

//go:generate mockgen -destination=./mock_renderer.go -package=renderers github.com/radius-project/radius/pkg/corerp/renderers Renderer
type Renderer interface {
	GetDependencyIDs(ctx context.Context, resource v1.DataModelInterface) (radiusResourceIDs []resources.ID, azureResourceIDs []resources.ID, err error)
	Render(ctx context.Context, resource v1.DataModelInterface, options RenderOptions) (RendererOutput, error)
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
	Resource v1.DataModelInterface

	// ComputedValues is a map of the computed values and secrets of the dependency.
	ComputedValues map[string]any

	// OutputResources is a map of the output resource IDs of the dependency. The map is keyed on the LocalID of the output resource.
	OutputResources map[string]resources.ID
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
	Identity *rpv1.IdentitySettings
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
	Resources      []rpv1.OutputResource
	ComputedValues map[string]rpv1.ComputedValueReference
	SecretValues   map[string]rpv1.SecretValueReference

	// RadiusResource is the original Radius resource model.
	RadiusResource v1.DataModelInterface
}
