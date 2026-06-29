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
// For secrets created before retain-at-rest landed (whose stored value is nil), the loader falls back to
// reading the backing Kubernetes Secret from the control-plane cluster — the previous behavior. That
// fallback is single-cluster only and is expected to be removed once existing secrets have been redeployed.
package secretsloader

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/crypto/encryption"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/resources"
	resources_kubernetes "github.com/radius-project/radius/pkg/ucp/resources/kubernetes"
)

const (
	// secretResourceType is the resource type of the user-defined secret resource.
	secretResourceType = "Radius.Security/secrets"
	// kubernetesSecretType is the output resource type of a Kubernetes Secret.
	kubernetesSecretType = "core/Secret"
)

var _ configloader.SecretsLoader = (*dispatchingLoader)(nil)
var _ configloader.SecretsLoader = (*udtSecretsLoader)(nil)

// NewDispatchingLoader returns a configloader.SecretsLoader that routes each secret ID to the appropriate
// backing loader based on its resource type: Radius.Security/secrets is resolved by decrypting the retained
// value from the Radius store (with a Kubernetes Secret fallback for pre-retain secrets), and all other
// types (e.g. Applications.Core/secretStores) are delegated to storeLoader.
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
// in the Radius store, falling back to the backing Kubernetes Secret for secrets created before retain.
type udtSecretsLoader struct {
	databaseClient database.Client
	kubeProvider   *kubernetesclientprovider.KubernetesClientProvider
	ucpClient      *v20231001preview.ClientFactory
}

// LoadSecrets reads each requested Radius.Security/secrets resource's live Kubernetes Secret and returns its data.
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

// loadSecret resolves a single Radius.Security/secrets resource to its cleartext data. It fails closed: if
// the secret cannot be resolved, an error is returned rather than partial/empty data.
//
// The value is retained encrypted at rest, so it is resolved by decrypting the stored resource with the
// control-plane key. Secrets created before retain landed have a nil value at rest; for those, loadSecret
// falls back to reading the backing Kubernetes Secret from the control-plane cluster.
func (l *udtSecretsLoader) loadSecret(ctx context.Context, secretID string, keysFilter []string) (recipes.SecretData, error) {
	if l.databaseClient == nil || l.kubeProvider == nil {
		return recipes.SecretData{}, fmt.Errorf("secret loader is not fully configured for Radius.Security/secrets")
	}

	resource, err := database.GetResource[datamodel.DynamicResource](ctx, l.databaseClient, secretID)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to get secret resource %q: %w", secretID, err)
	}

	// Resolve from the encrypted store copy first (multi-cluster safe).
	data, legacy, err := l.loadSecretFromStore(ctx, secretID, resource, keysFilter)
	if err != nil {
		return recipes.SecretData{}, err
	}
	if !legacy {
		return data, nil
	}

	// Migration fallback: the secret predates retain-at-rest (its value is nil in the store), so read the
	// backing Kubernetes Secret from the control-plane cluster. Single-cluster only.
	return l.loadSecretFromKubernetes(ctx, secretID, resource, keysFilter)
}

// loadSecretFromStore decrypts the secret resource's retained value using the control-plane encryption key
// and returns its cleartext. The returned legacy flag is true when the value is not retained at rest (the
// secret was created before retain landed, or no schema is available), signaling the Kubernetes fallback.
func (l *udtSecretsLoader) loadSecretFromStore(ctx context.Context, secretID string, resource *datamodel.DynamicResource, keysFilter []string) (recipes.SecretData, bool, error) {
	if resource.Properties == nil {
		return recipes.SecretData{}, true, nil
	}

	apiVersion := resource.InternalMetadata.UpdatedAPIVersion
	schemaMap, err := schema.GetSchema(ctx, l.ucpClient, secretID, secretResourceType, apiVersion)
	if err != nil {
		return recipes.SecretData{}, false, fmt.Errorf("failed to fetch schema for secret %q: %w", secretID, err)
	}
	if schemaMap == nil {
		// No schema available (e.g. ucpClient not configured); cannot decrypt. Defer to the fallback.
		return recipes.SecretData{}, true, nil
	}

	sensitivePaths := schema.ExtractSensitiveFieldPaths(schemaMap, "")

	runtimeClient, err := l.kubeProvider.RuntimeClient()
	if err != nil {
		return recipes.SecretData{}, false, fmt.Errorf("failed to create Kubernetes client to decrypt secret %q: %w", secretID, err)
	}

	// The encryption key lives in radius-system on the control-plane cluster, so decryption never depends
	// on the target cluster the application is deployed to.
	keyProvider := encryption.NewKubernetesKeyProvider(runtimeClient, nil)
	handler, err := encryption.NewSensitiveDataHandlerFromProvider(ctx, keyProvider)
	if err != nil {
		return recipes.SecretData{}, false, fmt.Errorf("failed to create decryption handler for secret %q: %w", secretID, err)
	}

	if err := handler.DecryptSensitiveFieldsWithSchema(ctx, resource.Properties, sensitivePaths, secretID, schemaMap); err != nil {
		return recipes.SecretData{}, false, fmt.Errorf("failed to decrypt secret %q: %w", secretID, err)
	}

	return buildSecretDataFromStore(secretID, resource.Properties, keysFilter)
}

// loadSecretFromKubernetes reads the secret's backing Kubernetes Secret from the control-plane cluster. This
// is the migration fallback for secrets created before retain-at-rest landed and is single-cluster only.
func (l *udtSecretsLoader) loadSecretFromKubernetes(ctx context.Context, secretID string, resource *datamodel.DynamicResource, keysFilter []string) (recipes.SecretData, error) {
	namespace, name, found := kubernetesSecretLocation(resource.OutputResources())
	if !found {
		return recipes.SecretData{}, fmt.Errorf("secret %q has no retained value and no Kubernetes Secret output resource; redeploy the secret to populate its encrypted value", secretID)
	}

	runtimeClient, err := l.kubeProvider.RuntimeClient()
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to create Kubernetes client to read secret %q: %w", secretID, err)
	}

	ksecret := &corev1.Secret{}
	if err := runtimeClient.Get(ctx, runtimeclient.ObjectKey{Namespace: namespace, Name: name}, ksecret); err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to read Kubernetes Secret for secret %q: %w", secretID, err)
	}

	return buildSecretData(secretID, ksecret, keysFilter)
}

// kubernetesSecretLocation returns the namespace and name of the Kubernetes Secret output resource, if present.
func kubernetesSecretLocation(outputResources []rpv1.OutputResource) (namespace string, name string, found bool) {
	for _, outputResource := range outputResources {
		if strings.EqualFold(outputResource.ID.Type(), kubernetesSecretType) {
			_, _, namespace, name = resources_kubernetes.ToParts(outputResource.ID)
			return namespace, name, true
		}
	}

	return "", "", false
}

// buildSecretData converts the live Kubernetes Secret into recipes.SecretData. When keysFilter is empty, all
// keys are returned; otherwise only the requested keys are returned and a missing key is an error.
func buildSecretData(secretID string, ksecret *corev1.Secret, keysFilter []string) (recipes.SecretData, error) {
	data := recipes.SecretData{
		Type: secretResourceType,
		Data: map[string]string{},
	}

	keys := keysFilter
	if len(keys) == 0 {
		keys = make([]string, 0, len(ksecret.Data))
		for key := range ksecret.Data {
			keys = append(keys, key)
		}
	}

	for _, key := range keys {
		value, ok := ksecret.Data[key]
		if !ok {
			return recipes.SecretData{}, fmt.Errorf("secret key %q was not found in secret %q", key, secretID)
		}
		data.Data[key] = string(value)
	}

	return data, nil
}

// buildSecretDataFromStore converts a decrypted Radius.Security/secrets properties map into recipes.SecretData.
// The secret's data property is a map of key to {value, encoding}; the (already decrypted) value is returned
// as-is, matching how Applications.Core/secretStores secrets are surfaced to recipes.
//
// When keysFilter is empty, all keys are returned; otherwise only the requested keys are returned and a
// missing key is an error. A requested key whose value is nil indicates the secret predates retain-at-rest
// (the backend redacted it), so the legacy flag is returned true to trigger the Kubernetes fallback.
func buildSecretDataFromStore(secretID string, properties map[string]any, keysFilter []string) (recipes.SecretData, bool, error) {
	rawData, ok := properties["data"].(map[string]any)
	if !ok {
		// No data to read from the store; defer to the Kubernetes fallback.
		return recipes.SecretData{}, true, nil
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
			return recipes.SecretData{}, false, fmt.Errorf("secret key %q was not found in secret %q", key, secretID)
		}

		entry, ok := entryRaw.(map[string]any)
		if !ok {
			return recipes.SecretData{}, false, fmt.Errorf("secret %q key %q has an unexpected format", secretID, key)
		}

		value, exists := entry["value"]
		if !exists || value == nil {
			// Value is redacted/absent at rest: this secret predates retain. Trigger the Kubernetes fallback.
			return recipes.SecretData{}, true, nil
		}

		valueStr, ok := value.(string)
		if !ok {
			return recipes.SecretData{}, false, fmt.Errorf("secret %q key %q value is not a string", secretID, key)
		}

		result.Data[key] = valueStr
	}

	return result, false, nil
}
