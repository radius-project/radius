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
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var _ SecretsLoader = (*secretsLoader)(nil)

// NewSecretStoreLoader creates a new SecretsLoader instance with the given ARM Client Options.
func NewSecretStoreLoader(armOptions *arm.ClientOptions) SecretsLoader {
	return &secretsLoader{ArmClientOptions: armOptions}
}

// SecretsLoader struct provides functionality to get secret information from Application.Core/SecretStore resource.
type secretsLoader struct {
	ArmClientOptions *arm.ClientOptions
}

// LoadSecrets loads secrets from secret stores based on input map of provided secret store IDs and secret keys.
// It returns a map of secret data, where the keys are the secret store IDs and the values are maps of secret keys and their corresponding values.
func (e *secretsLoader) LoadSecrets(ctx context.Context, secretStoreIDResourceKeys map[string][]string) (secretData map[string]map[string]string, err error) {
	for secretStoreID, secretKeys := range secretStoreIDResourceKeys {
		secretStoreResourceID, err := resources.ParseResource(secretStoreID)
		if err != nil {
			return nil, err
		}

		client, err := v20231001preview.NewSecretStoresClient(secretStoreResourceID.RootScope(), &aztoken.AnonymousCredential{}, e.ArmClientOptions)
		if err != nil {
			return nil, err
		}

		// Retrieve the secrets from the secret store.
		secrets, err := client.ListSecrets(ctx, secretStoreResourceID.Name(), map[string]any{}, nil)
		if err != nil {
			return nil, err
		}

		// Populate the secretData map.
		secretData, err = populateSecretData(secretStoreID, secretKeys, &secrets)
		if err != nil {
			return nil, err
		}
	}

	return secretData, nil
}

// populateSecretData is a helper function to populate secret data from a secret store.
func populateSecretData(secretStoreID string, secretKeys []string, secrets *v20231001preview.SecretStoresClientListSecretsResponse) (map[string]map[string]string, error) {
	secretData := make(map[string]map[string]string)

	if secrets == nil {
		return nil, fmt.Errorf("secrets not found for secret store ID '%s'", secretStoreID)
	}

	for _, secretKey := range secretKeys {
		if secretDataValue, ok := secrets.Data[secretKey]; ok {
			if secretData[secretStoreID] == nil {
				secretData[secretStoreID] = make(map[string]string)
			}
			secretData[secretStoreID][secretKey] = *secretDataValue.Value
		} else {
			return nil, fmt.Errorf("a secret key was not found in secret store '%s'", secretStoreID)
		}
	}

	return secretData, nil
}
