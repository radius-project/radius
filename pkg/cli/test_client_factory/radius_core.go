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

package test_client_factory

import (
	"context"
	"net/http"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	"github.com/radius-project/radius/pkg/to"
)

// NewRadiusCoreTestClientFactory creates a new client factory for testing purposes.
func NewRadiusCoreTestClientFactory(rootScope string, envServer func() corerpfake.EnvironmentsServer, recipepackServer func() corerpfake.RecipePacksServer) (*v20250801preview.ClientFactory, error) {
	serverFactory := corerpfake.ServerFactory{}
	if envServer != nil {
		serverFactory.EnvironmentsServer = envServer()
	} else {
		serverFactory.EnvironmentsServer = WithEnvironmentServerNoError()
	}

	if recipepackServer != nil {
		serverFactory.RecipePacksServer = recipepackServer()
	} else {
		serverFactory.RecipePacksServer = WithRecipePackServerNoError()
	}

	serverFactoryTransport := corerpfake.NewServerFactoryTransport(&serverFactory)

	clientOptions := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: serverFactoryTransport,
		},
	}

	clientFactory, err := v20250801preview.NewClientFactory(rootScope, &azfake.TokenCredential{}, clientOptions)
	if err != nil {
		return nil, err
	}

	return clientFactory, nil
}

func WithRecipePackServerNoError() corerpfake.RecipePacksServer {
	return corerpfake.RecipePacksServer{
		Get: func(ctx context.Context, recipePackName string, options *v20250801preview.RecipePacksClientGetOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientGetResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name: to.Ptr(recipePackName),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes: map[string]*v20250801preview.RecipeDefinition{
							"test-recipe1": {
								RecipeLocation: to.Ptr("https://example.com/recipe1?ref=v0.1"),
								RecipeKind:     to.Ptr(v20250801preview.RecipeKindTerraform),
							},
							"test-recipe2": {
								RecipeLocation: to.Ptr("https://example.com/recipe2?ref=v0.1"),
								RecipeKind:     to.Ptr(v20250801preview.RecipeKindTerraform),
							},
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
	}
}

func WithEnvironmentServerNoError() corerpfake.EnvironmentsServer {
	return corerpfake.EnvironmentsServer{
		CreateOrUpdate: func(
			ctx context.Context,
			environmentName string,
			resource v20250801preview.EnvironmentResource,
			options *v20250801preview.EnvironmentsClientCreateOrUpdateOptions,
		) (resp azfake.Responder[v20250801preview.EnvironmentsClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.EnvironmentsClientCreateOrUpdateResponse{
				EnvironmentResource: resource,
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
		Get: func(
			ctx context.Context,
			environmentName string,
			options *v20250801preview.EnvironmentsClientGetOptions,
		) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.EnvironmentsClientGetResponse{
				EnvironmentResource: v20250801preview.EnvironmentResource{
					Name: to.Ptr(environmentName),
					Properties: &v20250801preview.EnvironmentProperties{
						RecipePacks: []*string{
							to.Ptr("/planes/radius/local/resourceGroups/test-group/providers/Radius.Core/recipePacks/test-recipe-pack"),
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
		NewListByScopePager: func(options *v20250801preview.EnvironmentsClientListByScopeOptions) (resp azfake.PagerResponder[v20250801preview.EnvironmentsClientListByScopeResponse]) {
			resp.AddPage(
				http.StatusOK,
				v20250801preview.EnvironmentsClientListByScopeResponse{
					EnvironmentResourceListResult: v20250801preview.EnvironmentResourceListResult{
						Value: []*v20250801preview.EnvironmentResource{
							{
								Name: to.Ptr("test-env-1"),
							},
							{
								Name: to.Ptr("test-env-2"),
							},
						},
					},
				},
				nil,
			)
			return
		},
		Delete: func(
			ctx context.Context,
			environmentName string,
			options *v20250801preview.EnvironmentsClientDeleteOptions,
		) (resp azfake.Responder[v20250801preview.EnvironmentsClientDeleteResponse], errResp azfake.ErrorResponder) {
			resp.SetResponse(http.StatusNoContent, v20250801preview.EnvironmentsClientDeleteResponse{}, nil)
			return
		},
	}
}
