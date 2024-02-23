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
	"github.com/radius-project/radius/pkg/ucp/resources"
)

// NewSecretStoreLoader creates a new SecretsLoader instance with the given ARM Client Options.
func NewSecretStoreLoader(armOptions *arm.ClientOptions) SecretsLoader {
	return SecretsLoader{ArmClientOptions: armOptions}
}

// SecretsLoader struct provides functionality to get secret information from Application.Core/SecretStore resource.
type SecretsLoader struct {
	ArmClientOptions *arm.ClientOptions
}

func (e *SecretsLoader) LoadSecrets(ctx context.Context, secretStore string) (v20231001preview.SecretStoresClientListSecretsResponse, error) {
	secretStoreID, err := resources.ParseResource(secretStore)
	if err != nil {
		return v20231001preview.SecretStoresClientListSecretsResponse{}, err
	}

	client, err := v20231001preview.NewSecretStoresClient(secretStoreID.RootScope(), &aztoken.AnonymousCredential{}, e.ArmClientOptions)
	if err != nil {
		return v20231001preview.SecretStoresClientListSecretsResponse{}, err
	}

	secrets, err := client.ListSecrets(ctx, secretStoreID.Name(), map[string]any{}, nil)
	if err != nil {
		return v20231001preview.SecretStoresClientListSecretsResponse{}, err
	}

	return secrets, nil
}
