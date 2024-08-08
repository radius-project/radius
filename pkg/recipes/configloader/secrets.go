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

// LoadSecrets loads secrets from secret stores based on input map of provided secret store IDs and secret keys filter.
// If the input secret keys filter for a secret store ID is nil or empty, it retrieves secret data for all keys for that secret store.
// When the secret keys filter is populated for a secret store ID, it retrieves secret data for the specified keys for the associated secret store ID.
// The function returns a map of secret data, where the keys are the secret store IDs and the values are maps of secret keys and their corresponding values.
func (e *secretsLoader) LoadSecrets(ctx context.Context, secretStoreIDKeysFilter map[string][]string) (secretData map[string]map[string]string, err error) {
	secretData = make(map[string]map[string]string)

	for secretStoreID, secretKeysFilter := range secretStoreIDKeysFilter {
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
		secretStoreData, err := populateSecretData(secretStoreID, secretKeysFilter, &secrets)
		if err != nil {
			return nil, err
		}

		// Merge the secret data for current secret store id into returned secretData map.
		for key, value := range secretStoreData {
			secretData[key] = value
		}
	}

	return secretData, nil
}

// populateSecretData is a helper function to populate secret data from a secret store.
// It takes a secret store ID, a filter for secret keys, and a response containing the secrets.
// If the secret keys filter is nil or empty, it retrieves data for all keys in the secret store.
// It returns a map where the keys are the secret store IDs and the values are maps of secret keys and their corresponding values.
func populateSecretData(secretStoreID string, secretKeysFilter []string, secrets *v20231001preview.SecretStoresClientListSecretsResponse) (map[string]map[string]string, error) {
	if secrets == nil {
		return nil, fmt.Errorf("secrets not found for secret store ID '%s'", secretStoreID)
	}

	secretData := make(map[string]map[string]string)

	// If secretKeys is nil or empty, retrieve data for all keys
	if len(secretKeysFilter) == 0 {
		secretKeysFilter = make([]string, 0, len(secrets.Data))
		for secretKey := range secrets.Data {
			secretKeysFilter = append(secretKeysFilter, secretKey)
		}
	}

	for _, secretKey := range secretKeysFilter {
		secretDataValue, ok := secrets.Data[secretKey]
		if !ok {
			return nil, fmt.Errorf("a secret key was not found in secret store '%s'", secretStoreID)
		}

		if secretData[secretStoreID] == nil {
			secretData[secretStoreID] = make(map[string]string)
		}
		secretData[secretStoreID][secretKey] = *secretDataValue.Value
	}

	return secretData, nil
}
