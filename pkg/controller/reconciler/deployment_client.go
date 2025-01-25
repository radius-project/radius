/*
Copyright 2023.

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

package reconciler

import (
	"context"
	"fmt"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	sdkclients "github.com/radius-project/radius/pkg/sdk/clients"
)

type DeploymentClient interface {
	ResourceDeployments() ResourceDeploymentsClient
}

type ResourceDeploymentsClient interface {
	CreateOrUpdate(ctx context.Context, parameters sdkclients.Deployment, resourceID, apiVersion string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error)
	ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error)
	Delete(ctx context.Context, resourceID, apiVersion string) (Poller[sdkclients.ClientDeleteResponse], error)
	ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientDeleteResponse], error)
}

type DeploymentClientImpl struct {
	connection sdk.Connection
}

func NewDeploymentClient(connection sdk.Connection) *DeploymentClientImpl {
	return &DeploymentClientImpl{connection: connection}
}

var _ DeploymentClient = (*DeploymentClientImpl)(nil)

func (rdc *ResourceDeploymentsClientImpl) CreateOrUpdate(ctx context.Context, parameters sdkclients.Deployment, resourceID, apiVersion string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	return rdc.inner.CreateOrUpdate(ctx, parameters, resourceID, apiVersion)
}

func (rdc *ResourceDeploymentsClientImpl) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientCreateOrUpdateResponse], error) {
	return rdc.inner.ContinueCreateOperation(ctx, resumeToken)
}

func (rdc *ResourceDeploymentsClientImpl) Delete(ctx context.Context, resourceID, apiVersion string) (Poller[sdkclients.ClientDeleteResponse], error) {
	return rdc.inner.Delete(ctx, resourceID, apiVersion)
}

func (rdc *ResourceDeploymentsClientImpl) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[sdkclients.ClientDeleteResponse], error) {
	return rdc.inner.ContinueDeleteOperation(ctx, resumeToken)
}

var _ ResourceDeploymentsClient = (*ResourceDeploymentsClientImpl)(nil)

type ResourceDeploymentsClientImpl struct {
	inner sdkclients.ResourceDeploymentsClient
}

func (c *DeploymentClientImpl) ResourceDeployments() ResourceDeploymentsClient {
	rdc, err := sdkclients.NewResourceDeploymentsClient(&sdkclients.Options{
		Cred:             &aztoken.AnonymousCredential{},
		BaseURI:          c.connection.Endpoint(),
		ARMClientOptions: sdk.NewClientOptions(c.connection),
	})
	if err != nil {
		panic(fmt.Errorf("failed to create client: %w", err))
	}

	return &ResourceDeploymentsClientImpl{inner: rdc}
}
