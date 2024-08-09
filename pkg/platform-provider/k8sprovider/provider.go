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

package k8sprovider

import (
	"context"

	"github.com/radius-project/radius/pkg/corerp/datamodel"
	platformprovider "github.com/radius-project/radius/pkg/platform-provider"
)

func init() {
	platformprovider.Register("kubernetes", func() (platformprovider.Provider, error) {
		return &KubeProvider{}, nil
	})
}

var _ platformprovider.Provider = (*KubeProvider)(nil)
var _ platformprovider.ContainerProvider = (*KubeProvider)(nil)

type KubeProvider struct {
}

func (p *KubeProvider) Initialize() error {
	return nil
}

func (p *KubeProvider) Name() string {
	return "kubernetes"
}

func (p *KubeProvider) Container(name string) (platformprovider.ContainerProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Route(name string) (platformprovider.RouteProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Gateway(name string) (platformprovider.GatewayProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Identity(name string) (platformprovider.IdentityProvider, error) {
	return nil, nil
}

func (p *KubeProvider) Volume(name string) (platformprovider.VolumeProvider, error) {
	return nil, nil
}

func (p *KubeProvider) SecretStore(name string) (platformprovider.SecretStoreProvider, error) {
	return nil, nil
}

func (p *KubeProvider) CreateOrUpdateContainer(ctx context.Context, container *datamodel.ContainerResource) error {
	return nil
}
