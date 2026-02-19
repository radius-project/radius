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
	"fmt"
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
		CreateOrUpdate: func(ctx context.Context, recipePackName string, resource v20250801preview.RecipePackResource, options *v20250801preview.RecipePacksClientCreateOrUpdateOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientCreateOrUpdateResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name:       to.Ptr(recipePackName),
					Properties: resource.Properties,
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
						Providers: &v20250801preview.Providers{
							Azure: &v20250801preview.ProvidersAzure{
								SubscriptionID:    to.Ptr("test-subscription-id"),
								ResourceGroupName: to.Ptr("test-resource-group"),
							},
							Aws: &v20250801preview.ProvidersAws{
								AccountID: to.Ptr("test-account-id"),
								Region:    to.Ptr("test-region"),
							},
							Kubernetes: &v20250801preview.ProvidersKubernetes{
								Namespace: to.Ptr("test-namespace"),
							},
						},
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

// WithEnvironmentServer404OnGet returns an EnvironmentsServer that returns 404 on Get
// and success on CreateOrUpdate, simulating a new environment creation scenario.
func WithEnvironmentServer404OnGet() corerpfake.EnvironmentsServer {
	return corerpfake.EnvironmentsServer{
		Get: func(
			ctx context.Context,
			environmentName string,
			options *v20250801preview.EnvironmentsClientGetOptions,
		) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
			errResp.SetError(fmt.Errorf("environment not found"))
			errResp.SetResponseError(404, "Not Found")
			return
		},
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
	}
}

// WithEnvironmentServerNoRecipePacks returns an EnvironmentsServer that returns an existing
// environment with no recipe packs on Get, and success on CreateOrUpdate.
func WithEnvironmentServerNoRecipePacks() corerpfake.EnvironmentsServer {
	return corerpfake.EnvironmentsServer{
		Get: func(
			ctx context.Context,
			environmentName string,
			options *v20250801preview.EnvironmentsClientGetOptions,
		) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.EnvironmentsClientGetResponse{
				EnvironmentResource: v20250801preview.EnvironmentResource{
					Name:     to.Ptr(environmentName),
					Location: to.Ptr("global"),
					Properties: &v20250801preview.EnvironmentProperties{
						RecipePacks: []*string{},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
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
	}
}

// WithEnvironmentServerCustomRecipePacks returns a factory function that creates an EnvironmentsServer
// with the given recipe pack IDs on Get, and success on CreateOrUpdate.
func WithEnvironmentServerCustomRecipePacks(recipePacks []*string) func() corerpfake.EnvironmentsServer {
	return func() corerpfake.EnvironmentsServer {
		return corerpfake.EnvironmentsServer{
			Get: func(
				ctx context.Context,
				environmentName string,
				options *v20250801preview.EnvironmentsClientGetOptions,
			) (resp azfake.Responder[v20250801preview.EnvironmentsClientGetResponse], errResp azfake.ErrorResponder) {
				result := v20250801preview.EnvironmentsClientGetResponse{
					EnvironmentResource: v20250801preview.EnvironmentResource{
						Name:     to.Ptr(environmentName),
						Location: to.Ptr("global"),
						Properties: &v20250801preview.EnvironmentProperties{
							RecipePacks: recipePacks,
						},
					},
				}
				resp.SetResponse(http.StatusOK, result, nil)
				return
			},
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
		}
	}
}

// WithRecipePackServerCoreTypes returns a RecipePacksServer that maps singleton pack names
// to their actual core resource types. Non-singleton names get a unique test type.
func WithRecipePackServerCoreTypes() corerpfake.RecipePacksServer {
	// Build lookup from singleton definitions. These mirror the core
	// singleton recipe packs used by the CLI, but are duplicated here to
	// avoid importing the recipepack package and creating an import cycle in
	// tests.
	singletonTypes := map[string]string{
		"containers":        "Radius.Compute/containers",
		"persistentvolumes": "Radius.Compute/persistentVolumes",
		"routes":            "Radius.Compute/routes",
		"secrets":           "Radius.Security/secrets",
	}

	return corerpfake.RecipePacksServer{
		Get: func(ctx context.Context, recipePackName string, options *v20250801preview.RecipePacksClientGetOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
			resourceType, ok := singletonTypes[recipePackName]
			if !ok {
				resourceType = "Test.Resource/" + recipePackName
			}
			bicepKind := v20250801preview.RecipeKindBicep
			result := v20250801preview.RecipePacksClientGetResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name: to.Ptr(recipePackName),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes: map[string]*v20250801preview.RecipeDefinition{
							resourceType: {
								RecipeLocation: to.Ptr("ghcr.io/test/" + recipePackName + ":latest"),
								RecipeKind:     &bicepKind,
							},
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
		CreateOrUpdate: func(ctx context.Context, recipePackName string, resource v20250801preview.RecipePackResource, options *v20250801preview.RecipePacksClientCreateOrUpdateOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientCreateOrUpdateResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name:       to.Ptr(recipePackName),
					Properties: resource.Properties,
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
	}
}

// WithRecipePackServerUniqueTypes returns a RecipePacksServer where each pack name
// maps to a unique resource type based on the pack name.
func WithRecipePackServerUniqueTypes() corerpfake.RecipePacksServer {
	return corerpfake.RecipePacksServer{
		Get: func(ctx context.Context, recipePackName string, options *v20250801preview.RecipePacksClientGetOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
			bicepKind := v20250801preview.RecipeKindBicep
			result := v20250801preview.RecipePacksClientGetResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name: to.Ptr(recipePackName),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes: map[string]*v20250801preview.RecipeDefinition{
							"Test.Resource/" + recipePackName: {
								RecipeLocation: to.Ptr("ghcr.io/test/" + recipePackName + ":latest"),
								RecipeKind:     &bicepKind,
							},
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
		CreateOrUpdate: func(ctx context.Context, recipePackName string, resource v20250801preview.RecipePackResource, options *v20250801preview.RecipePacksClientCreateOrUpdateOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientCreateOrUpdateResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name:       to.Ptr(recipePackName),
					Properties: resource.Properties,
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
	}
}

// WithRecipePackServer404OnGet returns a RecipePacksServer that returns 404 on Get
// and success on CreateOrUpdate, simulating a scenario where recipe packs don't exist
// yet and need to be created.
func WithRecipePackServer404OnGet() corerpfake.RecipePacksServer {
	return corerpfake.RecipePacksServer{
		Get: func(ctx context.Context, recipePackName string, options *v20250801preview.RecipePacksClientGetOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
			errResp.SetError(fmt.Errorf("recipe pack not found"))
			errResp.SetResponseError(404, "Not Found")
			return
		},
		CreateOrUpdate: func(ctx context.Context, recipePackName string, resource v20250801preview.RecipePackResource, options *v20250801preview.RecipePacksClientCreateOrUpdateOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientCreateOrUpdateResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name:       to.Ptr(recipePackName),
					Properties: resource.Properties,
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
	}
}

// WithRecipePackServerConflictingTypes returns a RecipePacksServer where every pack
// returns the same resource type, simulating a conflict scenario.
func WithRecipePackServerConflictingTypes() corerpfake.RecipePacksServer {
	return corerpfake.RecipePacksServer{
		Get: func(ctx context.Context, recipePackName string, options *v20250801preview.RecipePacksClientGetOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientGetResponse], errResp azfake.ErrorResponder) {
			bicepKind := v20250801preview.RecipeKindBicep
			result := v20250801preview.RecipePacksClientGetResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name: to.Ptr(recipePackName),
					Properties: &v20250801preview.RecipePackProperties{
						Recipes: map[string]*v20250801preview.RecipeDefinition{
							"Radius.Compute/containers": {
								RecipeLocation: to.Ptr("ghcr.io/test/" + recipePackName + ":latest"),
								RecipeKind:     &bicepKind,
							},
						},
					},
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
		CreateOrUpdate: func(ctx context.Context, recipePackName string, resource v20250801preview.RecipePackResource, options *v20250801preview.RecipePacksClientCreateOrUpdateOptions) (resp azfake.Responder[v20250801preview.RecipePacksClientCreateOrUpdateResponse], errResp azfake.ErrorResponder) {
			result := v20250801preview.RecipePacksClientCreateOrUpdateResponse{
				RecipePackResource: v20250801preview.RecipePackResource{
					Name:       to.Ptr(recipePackName),
					Properties: resource.Properties,
				},
			}
			resp.SetResponse(http.StatusOK, result, nil)
			return
		},
	}
}
