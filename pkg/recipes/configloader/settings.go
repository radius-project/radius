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
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// FetchTerraformSettings fetches a TerraformSettings resource using the provided settingsID and ClientOptions,
// and returns the TerraformSettingsResource or an error.
func FetchTerraformSettings(ctx context.Context, settingsID string, ucpOptions *arm.ClientOptions) (*v20250801preview.TerraformSettingsResource, error) {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return nil, err
	}

	client, err := v20250801preview.NewTerraformSettingsClient(id.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, id.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.TerraformSettingsResource, nil
}

// FetchBicepSettings fetches a BicepSettings resource using the provided settingsID and ClientOptions,
// and returns the BicepSettingsResource or an error.
func FetchBicepSettings(ctx context.Context, settingsID string, ucpOptions *arm.ClientOptions) (*v20250801preview.BicepSettingsResource, error) {
	id, err := resources.ParseResource(settingsID)
	if err != nil {
		return nil, err
	}

	client, err := v20250801preview.NewBicepSettingsClient(id.RootScope(), &aztoken.AnonymousCredential{}, ucpOptions)
	if err != nil {
		return nil, err
	}

	response, err := client.Get(ctx, id.Name(), nil)
	if err != nil {
		return nil, err
	}

	return &response.BicepSettingsResource, nil
}
