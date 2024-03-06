package k8sprovider

import (
	"context"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	platformprovider "github.com/radius-project/radius/pkg/platform-provider"
)

var _ platformprovider.Provider = (*KubeProvider)(nil)
var _ platformprovider.ContainerProvider = (*KubeProvider)(nil)

type KubeProvider struct {
}

func (p *KubeProvider) Initialize() error {
	return nil
}

func (p *KubeProvider) Name() string {
	return "k8s"
}

func (p *KubeProvider) Container() (platformprovider.ContainerProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Route() (platformprovider.RouteProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Gateway() (platformprovider.GatewayProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Identity() (platformprovider.IdentityProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Volume() (platformprovider.VolumeProvider, error) {
	return nil, nil
}

func (p *KubeProvider) SecretStore() (platformprovider.SecretStoreProvider, error) {
	return nil, nil
}

func (p *KubeProvider) CreateOrUpdateContainer(ctx context.Context, container *datamodel.ContainerResource) error {
	return nil
}
