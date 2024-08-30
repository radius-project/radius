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
	"github.com/radius-project/radius/pkg/recipes"
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
// It returns a map with keys as secret store IDs and corresponding SecretData{}.
// If the input secret keys filter for a secret store ID is nil or empty, it retrieves secret data for all keys for that secret store ID.
// Eg: secretStoreKeysFilter = {"SecretStoreID1": nil} or secretKeysFilter = {"SecretStoreID1": []} will retrieve data for all secrets from "SecretStoreID1".
// ---
// When the secret keys filter is populated for a secret store ID, it retrieves secret data for the specified keys for the associated secret store ID.
// The function returns a map of secret data, where the keys are the secret store IDs and the values are maps of secret keys and their corresponding values.
// Eg; secretStoreKeysFilter = {"SecretStoreID1": ["secretkey1", "secretkey2"]} will retrieve data for only "secretkey1" and "secretkey2" keys from "SecretStoreID1".
func (e *secretsLoader) LoadSecrets(ctx context.Context, secretStoreKeysFilter map[string][]string) (secretData map[string]recipes.SecretData, err error) {
	secretData = make(map[string]recipes.SecretData)

	for secretStoreID, secretKeysFilter := range secretStoreKeysFilter {
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

		// Populate secretStoreData with secret type and map of secret keys and values
		secretStoreData, err := populateSecretData(secretStoreID, secretKeysFilter, &secrets)
		if err != nil {
			return nil, err
		}

		secretData[secretStoreID] = secretStoreData
	}

	return secretData, nil
}

// populateSecretData is a helper function to populate secret data from a secret store.
// It takes a secret store ID, a filter for secret keys, and a response containing the secret data.
// It returns SecretData{} populated with secret Type and a map with secret keys and their corresponding values.
// ---
// If the secret keys filter is nil or empty, it retrieves data for all keys in the secret store.
// Eg: secretKeysFilter = nil or secretKeysFilter = [] will retrieve data for all secrets for secretStoreID.
// ---
// If the secret keys filter is populated, it retrieves data for the specified keys.
// Eg: secretKeysFilter = [secretkey1] will retrieve data for only 'secretkey1' key.
func populateSecretData(secretStoreID string, secretKeysFilter []string, secrets *v20231001preview.SecretStoresClientListSecretsResponse) (recipes.SecretData, error) {
	if secrets == nil {
		return recipes.SecretData{}, fmt.Errorf("secrets not found for secret store ID '%s'", secretStoreID)
	}

	if secrets.Type == nil {
		return recipes.SecretData{}, fmt.Errorf("secret store type is not set for secret store ID '%s'", secretStoreID)
	}

	secretData := recipes.SecretData{
		Type: string(*secrets.Type),
		Data: make(map[string]string),
	}

	// If secretKeysFilter is nil or empty, populate secretKeysFilter with all keys
	if len(secretKeysFilter) == 0 {
		secretKeysFilter = make([]string, 0, len(secrets.Data))
		for secretKey := range secrets.Data {
			secretKeysFilter = append(secretKeysFilter, secretKey)
		}
	}

	for _, secretKey := range secretKeysFilter {
		secretDataValue, ok := secrets.Data[secretKey]
		if !ok {
			return recipes.SecretData{}, fmt.Errorf("a secret key was not found in secret store '%s'", secretStoreID)
		}
		secretData.Data[secretKey] = *secretDataValue.Value
	}

	return secretData, nil
}
