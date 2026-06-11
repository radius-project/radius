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

	corerpv20231001 "github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

// The generated SDK clients take the root scope as a per-method parameter. The management
// client, however, creates a client per scope and treats the scope as fixed for the lifetime
// of that client. The scoped* adapters below bind a root scope to a generated client so that
// it satisfies the corresponding *ResourceClient interface, which omits the per-method scope.

// scopedApplicationsClient binds a root scope to the generated Applications.Core/applications client.
type scopedApplicationsClient struct {
	inner *corerpv20231001.ApplicationsClient
	scope string
}

func (c *scopedApplicationsClient) CreateOrUpdate(ctx context.Context, applicationName string, resource corerpv20231001.ApplicationResource, options *corerpv20231001.ApplicationsClientCreateOrUpdateOptions) (corerpv20231001.ApplicationsClientCreateOrUpdateResponse, error) {
	return c.inner.CreateOrUpdate(ctx, c.scope, applicationName, resource, options)
}

func (c *scopedApplicationsClient) Delete(ctx context.Context, applicationName string, options *corerpv20231001.ApplicationsClientDeleteOptions) (corerpv20231001.ApplicationsClientDeleteResponse, error) {
	return c.inner.Delete(ctx, c.scope, applicationName, options)
}

func (c *scopedApplicationsClient) Get(ctx context.Context, applicationName string, options *corerpv20231001.ApplicationsClientGetOptions) (corerpv20231001.ApplicationsClientGetResponse, error) {
	return c.inner.Get(ctx, c.scope, applicationName, options)
}

func (c *scopedApplicationsClient) NewListByScopePager(options *corerpv20231001.ApplicationsClientListByScopeOptions) *runtime.Pager[corerpv20231001.ApplicationsClientListByScopeResponse] {
	return c.inner.NewListByScopePager(c.scope, options)
}

func (c *scopedApplicationsClient) GetGraph(ctx context.Context, applicationName string, body map[string]any, options *corerpv20231001.ApplicationsClientGetGraphOptions) (corerpv20231001.ApplicationsClientGetGraphResponse, error) {
	return c.inner.GetGraph(ctx, c.scope, applicationName, corerpv20231001.GetGraphRequest{}, options)
}

// scopedEnvironmentsClient binds a root scope to the generated Applications.Core/environments client.
type scopedEnvironmentsClient struct {
	inner *corerpv20231001.EnvironmentsClient
	scope string
}

func (c *scopedEnvironmentsClient) CreateOrUpdate(ctx context.Context, environmentName string, resource corerpv20231001.EnvironmentResource, options *corerpv20231001.EnvironmentsClientCreateOrUpdateOptions) (corerpv20231001.EnvironmentsClientCreateOrUpdateResponse, error) {
	return c.inner.CreateOrUpdate(ctx, c.scope, environmentName, resource, options)
}

func (c *scopedEnvironmentsClient) Delete(ctx context.Context, environmentName string, options *corerpv20231001.EnvironmentsClientDeleteOptions) (corerpv20231001.EnvironmentsClientDeleteResponse, error) {
	return c.inner.Delete(ctx, c.scope, environmentName, options)
}

func (c *scopedEnvironmentsClient) Get(ctx context.Context, environmentName string, options *corerpv20231001.EnvironmentsClientGetOptions) (corerpv20231001.EnvironmentsClientGetResponse, error) {
	return c.inner.Get(ctx, c.scope, environmentName, options)
}

func (c *scopedEnvironmentsClient) NewListByScopePager(options *corerpv20231001.EnvironmentsClientListByScopeOptions) *runtime.Pager[corerpv20231001.EnvironmentsClientListByScopeResponse] {
	return c.inner.NewListByScopePager(c.scope, options)
}

func (c *scopedEnvironmentsClient) GetMetadata(ctx context.Context, environmentName string, body corerpv20231001.RecipeGetMetadata, options *corerpv20231001.EnvironmentsClientGetMetadataOptions) (corerpv20231001.EnvironmentsClientGetMetadataResponse, error) {
	return c.inner.GetMetadata(ctx, c.scope, environmentName, body, options)
}

// scopedRecipePacksClient binds a root scope to the generated Radius.Core/recipePacks client.
type scopedRecipePacksClient struct {
	inner *corerpv20250801.RecipePacksClient
	scope string
}

func (c *scopedRecipePacksClient) CreateOrUpdate(ctx context.Context, recipePackName string, resource corerpv20250801.RecipePackResource, options *corerpv20250801.RecipePacksClientCreateOrUpdateOptions) (corerpv20250801.RecipePacksClientCreateOrUpdateResponse, error) {
	return c.inner.CreateOrUpdate(ctx, c.scope, recipePackName, resource, options)
}

func (c *scopedRecipePacksClient) Delete(ctx context.Context, recipePackName string, options *corerpv20250801.RecipePacksClientDeleteOptions) (corerpv20250801.RecipePacksClientDeleteResponse, error) {
	return c.inner.Delete(ctx, c.scope, recipePackName, options)
}

func (c *scopedRecipePacksClient) Get(ctx context.Context, recipePackName string, options *corerpv20250801.RecipePacksClientGetOptions) (corerpv20250801.RecipePacksClientGetResponse, error) {
	return c.inner.Get(ctx, c.scope, recipePackName, options)
}

func (c *scopedRecipePacksClient) NewListByScopePager(options *corerpv20250801.RecipePacksClientListByScopeOptions) *runtime.Pager[corerpv20250801.RecipePacksClientListByScopeResponse] {
	return c.inner.NewListByScopePager(c.scope, options)
}

// scopedRadiusCoreEnvironmentsClient binds a root scope to the generated Radius.Core/environments client.
type scopedRadiusCoreEnvironmentsClient struct {
	inner *corerpv20250801.EnvironmentsClient
	scope string
}

func (c *scopedRadiusCoreEnvironmentsClient) CreateOrUpdate(ctx context.Context, environmentName string, resource corerpv20250801.EnvironmentResource, options *corerpv20250801.EnvironmentsClientCreateOrUpdateOptions) (corerpv20250801.EnvironmentsClientCreateOrUpdateResponse, error) {
	return c.inner.CreateOrUpdate(ctx, c.scope, environmentName, resource, options)
}

func (c *scopedRadiusCoreEnvironmentsClient) Delete(ctx context.Context, environmentName string, options *corerpv20250801.EnvironmentsClientDeleteOptions) (corerpv20250801.EnvironmentsClientDeleteResponse, error) {
	return c.inner.Delete(ctx, c.scope, environmentName, options)
}

func (c *scopedRadiusCoreEnvironmentsClient) Get(ctx context.Context, environmentName string, options *corerpv20250801.EnvironmentsClientGetOptions) (corerpv20250801.EnvironmentsClientGetResponse, error) {
	return c.inner.Get(ctx, c.scope, environmentName, options)
}

func (c *scopedRadiusCoreEnvironmentsClient) NewListByScopePager(options *corerpv20250801.EnvironmentsClientListByScopeOptions) *runtime.Pager[corerpv20250801.EnvironmentsClientListByScopeResponse] {
	return c.inner.NewListByScopePager(c.scope, options)
}
