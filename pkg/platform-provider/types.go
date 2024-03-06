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
	Container() (ContainerProvider, error)

	// Route returns the route interface.
	Route() (RouteProvider, error)

	// Gateway returns the gateway interface.
	Gateway() (GatewayProvider, error)

	// Identity returns the identity interface.
	Identity() (IdentityProvider, error)

	// Volume returns the volume interface.
	Volume() (VolumeProvider, error)

	// SecretStore returns the secret store interface.
	SecretStore() (SecretStoreProvider, error)
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
