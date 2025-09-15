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

package configloader

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	resources "github.com/radius-project/radius/pkg/ucp/resources"
)

// FetchRecipePack fetches a recipe pack resource using the provided recipePackID and ClientOptions,
// and returns the RecipePackResource or an error.
func FetchRecipePack(ctx context.Context, recipePackID string, ucpOptions *arm.ClientOptions) (*v20231001preview.RecipePackResource, error) {
	rpID, err := resources.ParseResource(recipePackID)
	if err != nil {
		return nil, err
	}

	client, err := v20231001preview.NewRecipePacksClient(rpID.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, rpID.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.RecipePackResource, nil
}

// ListRecipePacks fetches all recipe pack resources in the given scope using the provided ClientOptions,
// and returns a slice of RecipePackResource or an error.
func ListRecipePacks(ctx context.Context, scope string, ucpOptions *arm.ClientOptions) ([]*v20231001preview.RecipePackResource, error) {
	client, err := v20231001preview.NewRecipePacksClient(scope, &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	var recipePacks []*v20231001preview.RecipePackResource
	pager := client.NewListByScopePager(nil)

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		recipePacks = append(recipePacks, page.Value...)
	}

	return recipePacks, nil
}
