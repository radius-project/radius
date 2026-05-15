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
	"testing"

	armpolicy "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	corerpfake "github.com/radius-project/radius/pkg/corerp/api/v20250801preview/fake"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, rpSrv corerpfake.RecipePacksServer) *v20250801preview.RecipePacksClient {
	t.Helper()
	opts := &armpolicy.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Transport: corerpfake.NewServerFactoryTransport(&corerpfake.ServerFactory{
				RecipePacksServer: rpSrv,
			}),
		},
	}
	client, err := v20250801preview.NewRecipePacksClient("/planes/radius/local/resourceGroups/test-rg", &aztoken.AnonymousCredential{}, opts)
	require.NoError(t, err)
	return client
}

func TestWithRecipePackServerNoError_Get(t *testing.T) {
	client := newTestClient(t, WithRecipePackServerNoError())

	resp, err := client.Get(context.Background(), "my-pack", nil)
	require.NoError(t, err)
	require.Equal(t, "my-pack", *resp.Name)
	require.Len(t, resp.Properties.Recipes, 2)
	require.Equal(t, v20250801preview.RecipeKindTerraform, *resp.Properties.Recipes["test-recipe1"].Kind)
	require.Equal(t, "https://example.com/recipe1?ref=v0.1", *resp.Properties.Recipes["test-recipe1"].Location)
	require.Equal(t, v20250801preview.RecipeKindTerraform, *resp.Properties.Recipes["test-recipe2"].Kind)
	require.Equal(t, "https://example.com/recipe2?ref=v0.1", *resp.Properties.Recipes["test-recipe2"].Location)
}

func TestWithRecipePackServerCoreTypes_Get(t *testing.T) {
	client := newTestClient(t, WithRecipePackServerCoreTypes())

	resp, err := client.Get(context.Background(), "containers", nil)
	require.NoError(t, err)
	require.Equal(t, "containers", *resp.Name)

	recipe := resp.Properties.Recipes["Radius.Compute/containers"]
	require.NotNil(t, recipe)
	require.Equal(t, v20250801preview.RecipeKindBicep, *recipe.Kind)
	require.Equal(t, "ghcr.io/test/containers:latest", *recipe.Location)
}

func TestWithRecipePackServerUniqueTypes_Get(t *testing.T) {
	client := newTestClient(t, WithRecipePackServerUniqueTypes())

	resp, err := client.Get(context.Background(), "mypack", nil)
	require.NoError(t, err)

	recipe := resp.Properties.Recipes["Test.Resource/mypack"]
	require.NotNil(t, recipe)
	require.Equal(t, v20250801preview.RecipeKindBicep, *recipe.Kind)
	require.Equal(t, "ghcr.io/test/mypack:latest", *recipe.Location)
}

func TestWithRecipePackServerConflictingTypes_Get(t *testing.T) {
	client := newTestClient(t, WithRecipePackServerConflictingTypes())

	resp, err := client.Get(context.Background(), "pack-a", nil)
	require.NoError(t, err)

	recipe := resp.Properties.Recipes["Radius.Compute/containers"]
	require.NotNil(t, recipe)
	require.Equal(t, v20250801preview.RecipeKindBicep, *recipe.Kind)
	require.Equal(t, "ghcr.io/test/pack-a:latest", *recipe.Location)
}
