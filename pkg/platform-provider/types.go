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

package platformprovider

import (
	"context"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// Provider is the interface that must be implemented by a platform provider.
type Provider interface {
	Initialize() error

	// Name returns the name of the platform provider.
	Name() string

	// Container returns the container interface. Container represents container orchestration.
	Container(name string) (ContainerProvider, error)

	// Route returns the route interface.
	Route(name string) (RouteProvider, error)

	// Gateway returns the gateway interface.
	Gateway(name string) (GatewayProvider, error)

	// Identity returns the identity interface.
	Identity(name string) (IdentityProvider, error)

	// Volume returns the volume interface.
	Volume(name string) (VolumeProvider, error)

	// SecretStore returns the secret store interface.
	SecretStore(name string) (SecretStoreProvider, error)
}

type ContainerProvider interface {
	CreateOrUpdateContainer(ctx context.Context, container *datamodel.ContainerResource) error
}

type RouteProvider interface {
	CreateOrUpdateRoute(ctx context.Context) error
	DeleteRoute(ctx context.Context) error
}

type GatewayProvider interface {
	CreateOrUpdateGateway(ctx context.Context, gateway *datamodel.Gateway) error
}

type IdentityProvider interface {
	CreateOrUpdateIdentity(ctx context.Context) (*resources.ID, error)
	AssignRoleToIdentity(ctx context.Context) error
}

type VolumeProvider interface {
}

type SecretStoreProvider interface {
	CreateOrUpdateSecretStore(ctx context.Context, secretStore *datamodel.SecretStore) error
	DeleteSecretStore(ctx context.Context) error
}
