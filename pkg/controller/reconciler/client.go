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
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001preview "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/sdk"
	ucpv20231001preview "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

type Poller[T any] interface {
	Done() bool
	Poll(ctx context.Context) (*http.Response, error)
	Result(ctx context.Context) (T, error)
	ResumeToken() (string, error)
}

var _ Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse] = (*runtime.Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse])(nil)

type RadiusClient interface {
	Applications(scope string) ApplicationClient
	Containers(scope string) ContainerClient
	Environments(scope string) EnvironmentClient
	Groups(scope string) ResourceGroupClient
	Resources(scope string, resourceType string) ResourceClient
}

type ApplicationClient interface {
	CreateOrUpdate(ctx context.Context, applicationName string, resource corerpv20231001preview.ApplicationResource, options *corerpv20231001preview.ApplicationsClientCreateOrUpdateOptions) (corerpv20231001preview.ApplicationsClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientDeleteOptions) (corerpv20231001preview.ApplicationsClientDeleteResponse, error)
	Get(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientGetOptions) (corerpv20231001preview.ApplicationsClientGetResponse, error)
}

type ContainerClient interface {
	BeginCreateOrUpdate(ctx context.Context, containerName string, resource corerpv20231001preview.ContainerResource, options *corerpv20231001preview.ContainersClientBeginCreateOrUpdateOptions) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error)
	BeginDelete(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientBeginDeleteOptions) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error)
	ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error)
	ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error)
	Get(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientGetOptions) (corerpv20231001preview.ContainersClientGetResponse, error)
}

type EnvironmentClient interface {
	List(ctx context.Context, options *corerpv20231001preview.EnvironmentsClientListByScopeOptions) (corerpv20231001preview.EnvironmentsClientListByScopeResponse, error)
}

type ResourceGroupClient interface {
	CreateOrUpdate(ctx context.Context, resourceGroupName string, resource ucpv20231001preview.ResourceGroupResource, options *ucpv20231001preview.ResourceGroupsClientCreateOrUpdateOptions) (ucpv20231001preview.ResourceGroupsClientCreateOrUpdateResponse, error)
	Get(ctx context.Context, resourceGroupName string, options *ucpv20231001preview.ResourceGroupsClientGetOptions) (ucpv20231001preview.ResourceGroupsClientGetResponse, error)
}

type ResourceClient interface {
	BeginCreateOrUpdate(ctx context.Context, resourceName string, resource generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error)
	BeginDelete(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (Poller[generated.GenericResourcesClientDeleteResponse], error)
	ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error)
	ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientDeleteResponse], error)
	Get(ctx context.Context, resourceName string) (generated.GenericResourcesClientGetResponse, error)
	ListSecrets(ctx context.Context, resourceName string) (generated.GenericResourcesClientListSecretsResponse, error)
}

type Client struct {
	connection sdk.Connection
}

func NewClient(connection sdk.Connection) *Client {
	return &Client{connection: connection}
}

var _ RadiusClient = (*Client)(nil)

func (c *Client) Applications(scope string) ApplicationClient {
	ac, err := corerpv20231001preview.NewApplicationsClient(scope, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		panic("failed to create client: " + err.Error())
	}

	return &ApplicationClientImpl{inner: ac}
}

func (c *Client) Containers(scope string) ContainerClient {
	cc, err := corerpv20231001preview.NewContainersClient(scope, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		panic("failed to create client: " + err.Error())
	}

	return &ContainerClientImpl{inner: cc}
}

func (c *Client) Environments(scope string) EnvironmentClient {
	ec, err := corerpv20231001preview.NewEnvironmentsClient(scope, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		panic("failed to create client: " + err.Error())
	}

	return &EnvironmentClientImpl{inner: ec}
}

func (c *Client) Groups(scope string) ResourceGroupClient {
	rgc, err := ucpv20231001preview.NewResourceGroupsClient(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		panic("failed to create client: " + err.Error())
	}

	return &ResourceGroupClientImpl{inner: rgc, scope: scope}
}

func (c *Client) Resources(scope string, resourceType string) ResourceClient {
	gc, err := generated.NewGenericResourcesClient(scope, resourceType, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(c.connection))
	if err != nil {
		panic("failed to create client: " + err.Error())
	}

	return &ResourceClientImpl{inner: gc}
}

var _ ApplicationClient = (*ApplicationClientImpl)(nil)

type ApplicationClientImpl struct {
	inner *corerpv20231001preview.ApplicationsClient
}

func (ac *ApplicationClientImpl) CreateOrUpdate(ctx context.Context, applicationName string, resource corerpv20231001preview.ApplicationResource, options *corerpv20231001preview.ApplicationsClientCreateOrUpdateOptions) (corerpv20231001preview.ApplicationsClientCreateOrUpdateResponse, error) {
	return ac.inner.CreateOrUpdate(ctx, applicationName, resource, options)
}

func (ac *ApplicationClientImpl) Delete(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientDeleteOptions) (corerpv20231001preview.ApplicationsClientDeleteResponse, error) {
	return ac.inner.Delete(ctx, applicationName, options)
}

func (ac *ApplicationClientImpl) Get(ctx context.Context, applicationName string, options *corerpv20231001preview.ApplicationsClientGetOptions) (corerpv20231001preview.ApplicationsClientGetResponse, error) {
	return ac.inner.Get(ctx, applicationName, options)
}

var _ ContainerClient = (*ContainerClientImpl)(nil)

type ContainerClientImpl struct {
	inner *corerpv20231001preview.ContainersClient
}

func (cc *ContainerClientImpl) BeginCreateOrUpdate(ctx context.Context, containerName string, resource corerpv20231001preview.ContainerResource, options *corerpv20231001preview.ContainersClientBeginCreateOrUpdateOptions) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error) {
	return cc.inner.BeginCreateOrUpdate(ctx, containerName, resource, options)
}

func (cc *ContainerClientImpl) BeginDelete(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientBeginDeleteOptions) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error) {
	return cc.inner.BeginDelete(ctx, containerName, options)
}

func (cc *ContainerClientImpl) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientCreateOrUpdateResponse], error) {
	return cc.inner.BeginCreateOrUpdate(ctx, "", corerpv20231001preview.ContainerResource{}, &corerpv20231001preview.ContainersClientBeginCreateOrUpdateOptions{ResumeToken: resumeToken})
}

func (cc *ContainerClientImpl) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[corerpv20231001preview.ContainersClientDeleteResponse], error) {
	return cc.inner.BeginDelete(ctx, "", &corerpv20231001preview.ContainersClientBeginDeleteOptions{ResumeToken: resumeToken})
}

func (cc *ContainerClientImpl) Get(ctx context.Context, containerName string, options *corerpv20231001preview.ContainersClientGetOptions) (corerpv20231001preview.ContainersClientGetResponse, error) {
	return cc.inner.Get(ctx, containerName, options)
}

var _ EnvironmentClient = (*EnvironmentClientImpl)(nil)

type EnvironmentClientImpl struct {
	inner *corerpv20231001preview.EnvironmentsClient
}

func (ec *EnvironmentClientImpl) List(ctx context.Context, options *corerpv20231001preview.EnvironmentsClientListByScopeOptions) (corerpv20231001preview.EnvironmentsClientListByScopeResponse, error) {
	result := corerpv20231001preview.EnvironmentsClientListByScopeResponse{}
	pager := ec.inner.NewListByScopePager(options)
	for pager.More() {
		response, err := pager.NextPage(ctx)
		if err != nil {
			return corerpv20231001preview.EnvironmentsClientListByScopeResponse{}, err
		}

		result.Value = append(result.Value, response.Value...)
	}

	return result, nil
}

type ResourceGroupClientImpl struct {
	inner *ucpv20231001preview.ResourceGroupsClient
	scope string
}

func (rgc *ResourceGroupClientImpl) CreateOrUpdate(ctx context.Context, resourceGroupName string, resource ucpv20231001preview.ResourceGroupResource, options *ucpv20231001preview.ResourceGroupsClientCreateOrUpdateOptions) (ucpv20231001preview.ResourceGroupsClientCreateOrUpdateResponse, error) {
	parsed, err := resources.ParseScope(rgc.scope)
	if err != nil {
		return ucpv20231001preview.ResourceGroupsClientCreateOrUpdateResponse{}, err
	}

	return rgc.inner.CreateOrUpdate(ctx, parsed.FindScope("radius"), resourceGroupName, resource, options)
}

func (rgc *ResourceGroupClientImpl) Get(ctx context.Context, resourceGroupName string, options *ucpv20231001preview.ResourceGroupsClientGetOptions) (ucpv20231001preview.ResourceGroupsClientGetResponse, error) {
	parsed, err := resources.ParseScope(rgc.scope)
	if err != nil {
		return ucpv20231001preview.ResourceGroupsClientGetResponse{}, err
	}

	return rgc.inner.Get(ctx, parsed.FindScope("radius"), resourceGroupName, options)
}

var _ ResourceClient = (*ResourceClientImpl)(nil)

type ResourceClientImpl struct {
	inner *generated.GenericResourcesClient
}

func (rc *ResourceClientImpl) BeginCreateOrUpdate(ctx context.Context, resourceName string, resource generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	return rc.inner.BeginCreateOrUpdate(ctx, resourceName, resource, options)
}

func (rc *ResourceClientImpl) BeginDelete(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	return rc.inner.BeginDelete(ctx, resourceName, options)
}

func (rc *ResourceClientImpl) ContinueCreateOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error) {
	return rc.inner.BeginCreateOrUpdate(ctx, "", generated.GenericResource{}, &generated.GenericResourcesClientBeginCreateOrUpdateOptions{ResumeToken: resumeToken})
}

func (rc *ResourceClientImpl) ContinueDeleteOperation(ctx context.Context, resumeToken string) (Poller[generated.GenericResourcesClientDeleteResponse], error) {
	return rc.inner.BeginDelete(ctx, "", &generated.GenericResourcesClientBeginDeleteOptions{ResumeToken: resumeToken})
}

func (rc *ResourceClientImpl) Get(ctx context.Context, resourceName string) (generated.GenericResourcesClientGetResponse, error) {
	return rc.inner.Get(ctx, resourceName, nil)
}

func (rc *ResourceClientImpl) ListSecrets(ctx context.Context, resourceName string) (generated.GenericResourcesClientListSecretsResponse, error) {
	return rc.inner.ListSecrets(ctx, resourceName, nil)
}
