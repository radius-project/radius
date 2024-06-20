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

package clients

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	corerpv20231001 "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	ucpv20231001 "github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
)

// We define interfaces so we can mock the interactions with the Radius API. These
// mock interfaces describe the APIs called by the UCPApplicationsManagementClient.
//
// These interfaces match the generated API clients so that we can use them to mock
// the generated clients in our tests.
//
// Because these interfaces are non-exported, they MUST be defined in their own file
// and we MUST use -source on mockgen to generate mocks for them.

//go:generate mockgen -typed -source=./management_mocks.go -destination=./mock_management_wrapped_clients.go -package=clients -self_package github.com/radius-project/radius/pkg/cli/clients github.com/radius-project/radius/pkg/cli/clients genericResourceClient,applicationResourceClient,environmentResourceClient,resourceGroupClient,resourceProviderClient

// genericResourceClient is an interface for mocking the generated SDK client for any resource.
type genericResourceClient interface {
	BeginCreateOrUpdate(ctx context.Context, resourceName string, genericResourceParameters generated.GenericResource, options *generated.GenericResourcesClientBeginCreateOrUpdateOptions) (*runtime.Poller[generated.GenericResourcesClientCreateOrUpdateResponse], error)
	BeginDelete(ctx context.Context, resourceName string, options *generated.GenericResourcesClientBeginDeleteOptions) (*runtime.Poller[generated.GenericResourcesClientDeleteResponse], error)
	Get(ctx context.Context, resourceName string, options *generated.GenericResourcesClientGetOptions) (generated.GenericResourcesClientGetResponse, error)
	NewListByRootScopePager(options *generated.GenericResourcesClientListByRootScopeOptions) *runtime.Pager[generated.GenericResourcesClientListByRootScopeResponse]
}

// applicationResourceClient is an interface for mocking the generated SDK client for application resources.
type applicationResourceClient interface {
	CreateOrUpdate(ctx context.Context, applicationName string, resource corerpv20231001.ApplicationResource, options *corerpv20231001.ApplicationsClientCreateOrUpdateOptions) (corerpv20231001.ApplicationsClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, applicationName string, options *corerpv20231001.ApplicationsClientDeleteOptions) (corerpv20231001.ApplicationsClientDeleteResponse, error)
	Get(ctx context.Context, applicationName string, options *corerpv20231001.ApplicationsClientGetOptions) (corerpv20231001.ApplicationsClientGetResponse, error)
	NewListByScopePager(options *corerpv20231001.ApplicationsClientListByScopeOptions) *runtime.Pager[corerpv20231001.ApplicationsClientListByScopeResponse]

	GetGraph(ctx context.Context, applicationName string, body map[string]any, options *corerpv20231001.ApplicationsClientGetGraphOptions) (corerpv20231001.ApplicationsClientGetGraphResponse, error)
}

// environmentResourceClient is an interface for mocking the generated SDK client for environment resources.
type environmentResourceClient interface {
	CreateOrUpdate(ctx context.Context, environmentName string, resource corerpv20231001.EnvironmentResource, options *corerpv20231001.EnvironmentsClientCreateOrUpdateOptions) (corerpv20231001.EnvironmentsClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, environmentName string, options *corerpv20231001.EnvironmentsClientDeleteOptions) (corerpv20231001.EnvironmentsClientDeleteResponse, error)
	Get(ctx context.Context, environmentName string, options *corerpv20231001.EnvironmentsClientGetOptions) (corerpv20231001.EnvironmentsClientGetResponse, error)
	NewListByScopePager(options *corerpv20231001.EnvironmentsClientListByScopeOptions) *runtime.Pager[corerpv20231001.EnvironmentsClientListByScopeResponse]

	GetMetadata(ctx context.Context, environmentName string, body corerpv20231001.RecipeGetMetadata, options *corerpv20231001.EnvironmentsClientGetMetadataOptions) (corerpv20231001.EnvironmentsClientGetMetadataResponse, error)
}

// resourceGroupClient is an interface for mocking the generated SDK client for resource groups.
type resourceGroupClient interface {
	CreateOrUpdate(ctx context.Context, planeName string, resourceGroupName string, resource ucpv20231001.ResourceGroupResource, options *ucpv20231001.ResourceGroupsClientCreateOrUpdateOptions) (ucpv20231001.ResourceGroupsClientCreateOrUpdateResponse, error)
	Delete(ctx context.Context, planeName string, resourceGroupName string, options *ucpv20231001.ResourceGroupsClientDeleteOptions) (ucpv20231001.ResourceGroupsClientDeleteResponse, error)
	Get(ctx context.Context, planeName string, resourceGroupName string, options *ucpv20231001.ResourceGroupsClientGetOptions) (ucpv20231001.ResourceGroupsClientGetResponse, error)
	NewListPager(planeName string, options *ucpv20231001.ResourceGroupsClientListOptions) *runtime.Pager[ucpv20231001.ResourceGroupsClientListResponse]
}

// resourceProviderClient is an interface for mocking the generated SDK client for resource providers.
type resourceProviderClient interface {
	BeginCreateOrUpdate(ctx context.Context, planeName string, resourceProviderName string, resource ucpv20231001.ResourceProviderResource, options *ucpv20231001.ResourceProvidersClientBeginCreateOrUpdateOptions) (*runtime.Poller[ucpv20231001.ResourceProvidersClientCreateOrUpdateResponse], error)
	BeginDelete(ctx context.Context, planeName string, resourceProviderName string, options *ucpv20231001.ResourceProvidersClientBeginDeleteOptions) (*runtime.Poller[ucpv20231001.ResourceProvidersClientDeleteResponse], error)
	Get(ctx context.Context, planeName string, resourceProviderName string, options *ucpv20231001.ResourceProvidersClientGetOptions) (ucpv20231001.ResourceProvidersClientGetResponse, error)
	NewListPager(planeName string, options *ucpv20231001.ResourceProvidersClientListOptions) *runtime.Pager[ucpv20231001.ResourceProvidersClientListResponse]
}
