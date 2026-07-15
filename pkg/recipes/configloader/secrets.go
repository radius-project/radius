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
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/corerp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// secretStoresResourceType is the resource type of an Applications.Core/secretStores resource.
	secretStoresResourceType = "Applications.Core/secretStores"

	// securitySecretsResourceType is the resource type of a Radius.Security/secrets resource.
	securitySecretsResourceType = "Radius.Security/secrets"
)

var _ SecretsLoader = (*secretsLoader)(nil)

// NewSecretStoreLoader creates a new SecretsLoader instance with the given ARM Client Options and Kubernetes
// client provider. The Kubernetes client provider is used to read the backing Kubernetes Secret of a
// Radius.Security/secrets resource; it may be nil when only Applications.Core/secretStores references are expected.
func NewSecretStoreLoader(armOptions *arm.ClientOptions, k8sProvider *kubernetesclientprovider.KubernetesClientProvider) SecretsLoader {
	return &secretsLoader{ArmClientOptions: armOptions, KubernetesProvider: k8sProvider}
}

// SecretsLoader struct provides functionality to get secret information from Applications.Core/secretStores and
// Radius.Security/secrets resources.
type secretsLoader struct {
	ArmClientOptions *arm.ClientOptions

	// KubernetesProvider provides access to the Kubernetes client used to read the backing Secret of a
	// Radius.Security/secrets resource.
	KubernetesProvider *kubernetesclientprovider.KubernetesClientProvider
}

// LoadSecrets loads secrets from secret stores based on input map of provided secret store IDs and secret keys filter.
// It returns a map with keys as secret store IDs and corresponding SecretData{}.
// If the input secret keys filter for a secret store ID is nil or empty, it retrieves secret data for all keys for that secret store ID.
// Eg: secretStoreKeysFilter = {"SecretStoreID1": nil} or secretKeysFilter = {"SecretStoreID1": []} will retrieve data for all secrets from "SecretStoreID1".
// ---
// When the secret keys filter is populated for a secret store ID, it retrieves secret data for the specified keys for the associated secret store ID.
// The function returns a map of secret data, where the keys are the secret store IDs and the values are maps of secret keys and their corresponding values.
// Eg; secretStoreKeysFilter = {"SecretStoreID1": ["secretkey1", "secretkey2"]} will retrieve data for only "secretkey1" and "secretkey2" keys from "SecretStoreID1".
// ---
// The referenced secret resource may be either an Applications.Core/secretStores resource (used by legacy
// Applications.Core/environments) or a Radius.Security/secrets resource (used by Radius.Core/environments via
// bicepSettings/terraformSettings). The loader dispatches on the parsed resource type.
func (e *secretsLoader) LoadSecrets(ctx context.Context, secretStoreKeysFilter map[string][]string) (secretData map[string]recipes.SecretData, err error) {
	secretData = make(map[string]recipes.SecretData)

	for secretStoreID, secretKeysFilter := range secretStoreKeysFilter {
		secretStoreResourceID, err := resources.ParseResource(secretStoreID)
		if err != nil {
			return nil, err
		}

		var secretStoreData recipes.SecretData
		switch {
		case strings.EqualFold(secretStoreResourceID.Type(), securitySecretsResourceType):
			secretStoreData, err = e.loadSecuritySecret(ctx, secretStoreResourceID, secretKeysFilter)
		case strings.EqualFold(secretStoreResourceID.Type(), secretStoresResourceType):
			secretStoreData, err = e.loadSecretStore(ctx, secretStoreResourceID, secretKeysFilter)
		default:
			return nil, fmt.Errorf("unsupported secret resource type '%s' for secret '%s'", secretStoreResourceID.Type(), secretStoreID)
		}
		if err != nil {
			return nil, err
		}

		secretData[secretStoreID] = secretStoreData
	}

	return secretData, nil
}

// loadSecretStore retrieves secret data from an Applications.Core/secretStores resource using its ListSecrets API.
func (e *secretsLoader) loadSecretStore(ctx context.Context, secretStoreResourceID resources.ID, secretKeysFilter []string) (recipes.SecretData, error) {
	client, err := v20231001preview.NewSecretStoresClient(&aztoken.AnonymousCredential{}, e.ArmClientOptions)
	if err != nil {
		return recipes.SecretData{}, err
	}

	// Retrieve the secrets from the secret store.
	secrets, err := client.ListSecrets(ctx, secretStoreResourceID.RootScope(), secretStoreResourceID.Name(), v20231001preview.ListSecretsRequest{}, nil)
	if err != nil {
		return recipes.SecretData{}, err
	}

	// Populate secretStoreData with secret type and map of secret keys and values
	return populateSecretData(secretStoreResourceID.String(), secretKeysFilter, &secrets)
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
			return recipes.SecretData{}, fmt.Errorf(
				"'%s' secret key was not found in secret store '%s'",
				secretKey,
				secretStoreID,
			)
		}
		secretData.Data[secretKey] = *secretDataValue.Value
	}

	return secretData, nil
}
