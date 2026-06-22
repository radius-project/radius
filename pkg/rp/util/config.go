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

package util

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	resources "github.com/radius-project/radius/pkg/ucp/resources"
)

// FetchTerraformConfig fetches a TerraformConfig resource by ID.
func FetchTerraformConfig(ctx context.Context, resourceID string, ucpOptions *arm.ClientOptions) (*v20250801preview.TerraformConfigResource, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, err
	}

	client, err := v20250801preview.NewTerraformConfigsClient(&aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, id.RootScope(), id.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.TerraformConfigResource, nil
}

// FetchBicepConfig fetches a BicepConfig resource by ID.
func FetchBicepConfig(ctx context.Context, resourceID string, ucpOptions *arm.ClientOptions) (*v20250801preview.BicepConfigResource, error) {
	id, err := resources.ParseResource(resourceID)
	if err != nil {
		return nil, err
	}

	client, err := v20250801preview.NewBicepConfigsClient(&aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, id.RootScope(), id.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.BicepConfigResource, nil
}
