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

// Package secretsloader provides a configloader.SecretsLoader that can read Radius.Security/secrets
// resources in addition to the Applications.Core/secretStores resources handled by the default loader.
//
// A Radius.Security/secrets value is retained encrypted at rest (x-radius-retain): the backend keeps the
// frontend-encrypted value in the Radius store instead of redacting it to nil after recipe execution. This
// loader therefore resolves cleartext by decrypting the stored resource with the control-plane key (in
// radius-system), which works regardless of which cluster the application's Kubernetes Secret lives in
// (multi-cluster safe).
//
// The loader fails closed: if a secret's value is not retained encrypted at rest — for example a secret
// created before retain-at-rest landed, whose stored value is nil — it returns an error directing the
// operator to redeploy the secret, rather than silently falling back to reading the secret from a single
// cluster's Kubernetes Secret.
package secretsloader

import (
	"context"
	"fmt"
	"strings"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

const (
	// secretResourceType is the resource type of the user-defined secret resource.
	secretResourceType = "Radius.Security/secrets"
)

var _ configloader.SecretsLoader = (*dispatchingLoader)(nil)
var _ configloader.SecretsLoader = (*udtSecretsLoader)(nil)

// NewDispatchingLoader returns a configloader.SecretsLoader that routes each secret ID to the appropriate
// backing loader based on its resource type: Radius.Security/secrets is resolved by decrypting the retained
// value from the Radius store, and all other types (e.g. Applications.Core/secretStores) are delegated to
// storeLoader.
func NewDispatchingLoader(storeLoader configloader.SecretsLoader, databaseClient database.Client, kubeProvider *kubernetesclientprovider.KubernetesClientProvider, ucpClient *v20231001preview.ClientFactory) configloader.SecretsLoader {
	return &dispatchingLoader{
		storeLoader: storeLoader,
		udtLoader:   &udtSecretsLoader{databaseClient: databaseClient, kubeProvider: kubeProvider, ucpClient: ucpClient},
	}
}

// dispatchingLoader routes secret IDs to a type-specific loader.
type dispatchingLoader struct {
	storeLoader configloader.SecretsLoader
	udtLoader   configloader.SecretsLoader
}

// LoadSecrets partitions the requested secret IDs by resource type and delegates each partition to the
// loader that can read it, then merges the results.
func (l *dispatchingLoader) LoadSecrets(ctx context.Context, secretStoreIDs map[string][]string) (map[string]recipes.SecretData, error) {
	udtFilter := map[string][]string{}
	storeFilter := map[string][]string{}
	for id, keys := range secretStoreIDs {
		parsed, err := resources.ParseResource(id)
		if err != nil {
			return nil, fmt.Errorf("failed to parse secret resource ID %q: %w", id, err)
		}

		if strings.EqualFold(parsed.Type(), secretResourceType) {
			udtFilter[id] = keys
		} else {
			storeFilter[id] = keys
		}
	}

	result := map[string]recipes.SecretData{}

	if len(udtFilter) > 0 {
		loaded, err := l.udtLoader.LoadSecrets(ctx, udtFilter)
		if err != nil {
			return nil, err
		}
		for id, data := range loaded {
			result[id] = data
		}
	}

	if len(storeFilter) > 0 {
		if l.storeLoader == nil {
			return nil, fmt.Errorf("no secret store loader is configured to load secrets from %d secret store(s)", len(storeFilter))
		}
		loaded, err := l.storeLoader.LoadSecrets(ctx, storeFilter)
		if err != nil {
			return nil, err
		}
		for id, data := range loaded {
			result[id] = data
		}
	}

	return result, nil
}

// udtSecretsLoader reads cleartext for Radius.Security/secrets resources by decrypting the value retained
// encrypted in the Radius store with the control-plane key.
type udtSecretsLoader struct {
	databaseClient database.Client
	kubeProvider   *kubernetesclientprovider.KubernetesClientProvider
	ucpClient      *v20231001preview.ClientFactory
}

// LoadSecrets resolves each requested Radius.Security/secrets resource by decrypting its retained value from
// the Radius store and returns the cleartext data.
func (l *udtSecretsLoader) LoadSecrets(ctx context.Context, secretStoreIDs map[string][]string) (map[string]recipes.SecretData, error) {
	result := map[string]recipes.SecretData{}
	for id, keys := range secretStoreIDs {
		data, err := l.loadSecret(ctx, id, keys)
		if err != nil {
			return nil, err
		}
		result[id] = data
	}

	return result, nil
}

// loadSecret resolves a single Radius.Security/secrets resource to its cleartext data by decrypting the
// value retained encrypted in the Radius store with the control-plane key. It fails closed: if the secret
// cannot be resolved — including a secret created before retain-at-rest landed, whose value is nil at rest —
// an error is returned rather than partial/empty data or a silent fallback to a single-cluster read.
func (l *udtSecretsLoader) loadSecret(ctx context.Context, secretID string, keysFilter []string) (recipes.SecretData, error) {
	if l.databaseClient == nil || l.kubeProvider == nil {
		return recipes.SecretData{}, fmt.Errorf("secret loader is not fully configured for Radius.Security/secrets")
	}

	resource, err := database.GetResource[datamodel.DynamicResource](ctx, l.databaseClient, secretID)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to get secret resource %q: %w", secretID, err)
	}

	return l.loadSecretFromStore(ctx, secretID, resource, keysFilter)
}

// loadSecretFromStore decrypts the secret resource's retained value using the control-plane encryption key
// (held in radius-system) and returns its cleartext. Because the key lives on the control-plane cluster,
// decryption never depends on the application's target cluster, so it is multi-cluster safe. It fails closed
// if the value is not retained encrypted at rest.
func (l *udtSecretsLoader) loadSecretFromStore(ctx context.Context, secretID string, resource *datamodel.DynamicResource, keysFilter []string) (recipes.SecretData, error) {
	if resource.Properties == nil {
		return recipes.SecretData{}, fmt.Errorf("secret %q has no properties to resolve; redeploy the secret so its value is stored encrypted at rest", secretID)
	}

	apiVersion := resource.InternalMetadata.UpdatedAPIVersion
	schemaMap, err := schema.GetSchema(ctx, l.ucpClient, secretID, secretResourceType, apiVersion)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to fetch schema for secret %q: %w", secretID, err)
	}
	if schemaMap == nil {
		return recipes.SecretData{}, fmt.Errorf("no schema is available for secret %q (%s, api-version %q); cannot decrypt its retained value", secretID, secretResourceType, apiVersion)
	}

	sensitivePaths := schema.ExtractSensitiveFieldPaths(schemaMap, "")

	runtimeClient, err := l.kubeProvider.RuntimeClient()
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to create Kubernetes client to decrypt secret %q: %w", secretID, err)
	}

	// The encryption key lives in radius-system on the control-plane cluster, so decryption never depends
	// on the target cluster the application is deployed to.
	keyProvider := encryption.NewKubernetesKeyProvider(runtimeClient, nil)
	handler, err := encryption.NewSensitiveDataHandlerFromProvider(ctx, keyProvider)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to create decryption handler for secret %q: %w", secretID, err)
	}

	if err := handler.DecryptSensitiveFieldsWithSchema(ctx, resource.Properties, sensitivePaths, secretID, schemaMap); err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to decrypt secret %q: %w", secretID, err)
	}

	return buildSecretDataFromStore(secretID, resource.Properties, keysFilter)
}

// buildSecretDataFromStore converts a decrypted Radius.Security/secrets properties map into recipes.SecretData.
// The secret's data property is a map of key to {value, encoding}; the (already decrypted) value is returned
// as-is, matching how Applications.Core/secretStores secrets are surfaced to recipes.
//
// When keysFilter is empty, all keys are returned; otherwise only the requested keys are returned. It fails
// closed: a missing requested key, or a value that is not retained at rest (nil, e.g. a secret created before
// retain landed), is an error rather than empty/incorrect data.
func buildSecretDataFromStore(secretID string, properties map[string]any, keysFilter []string) (recipes.SecretData, error) {
	rawData, ok := properties["data"].(map[string]any)
	if !ok {
		return recipes.SecretData{}, fmt.Errorf("secret %q has no data stored at rest; redeploy the secret so its value is stored encrypted at rest", secretID)
	}

	keys := keysFilter
	if len(keys) == 0 {
		keys = make([]string, 0, len(rawData))
		for key := range rawData {
			keys = append(keys, key)
		}
	}

	result := recipes.SecretData{
		Type: secretResourceType,
		Data: map[string]string{},
	}

	for _, key := range keys {
		entryRaw, exists := rawData[key]
		if !exists {
			return recipes.SecretData{}, fmt.Errorf("secret key %q was not found in secret %q", key, secretID)
		}

		entry, ok := entryRaw.(map[string]any)
		if !ok {
			return recipes.SecretData{}, fmt.Errorf("secret %q key %q has an unexpected format", secretID, key)
		}

		value, exists := entry["value"]
		if !exists || value == nil {
			return recipes.SecretData{}, fmt.Errorf("secret %q key %q has no value stored at rest; it may have been created before secrets were retained encrypted at rest — redeploy the secret to populate its encrypted value", secretID, key)
		}

		valueStr, ok := value.(string)
		if !ok {
			return recipes.SecretData{}, fmt.Errorf("secret %q key %q value is not a string", secretID, key)
		}

		result.Data[key] = valueStr
	}

	return result, nil
}
