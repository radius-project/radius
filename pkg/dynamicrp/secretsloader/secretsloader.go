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
// Cleartext for a Radius.Security/secrets resource is never persisted to the database: the resource's
// sensitive data is redacted before recipe execution. The cleartext lives only in the Kubernetes Secret
// that the secret's own recipe materialized. This loader therefore reads the referenced secret's
// status.outputResources, locates the backing Kubernetes Secret, and reads the live values on demand.
package secretsloader

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/recipes/configloader"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
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
// backing loader based on its resource type: Radius.Security/secrets is read from its deployed Kubernetes
// Secret, and all other types (e.g. Applications.Core/secretStores) are delegated to storeLoader.
func NewDispatchingLoader(storeLoader configloader.SecretsLoader, databaseClient database.Client, kubeProvider *kubernetesclientprovider.KubernetesClientProvider) configloader.SecretsLoader {
	return &dispatchingLoader{
		storeLoader: storeLoader,
		udtLoader:   &udtSecretsLoader{databaseClient: databaseClient, kubeProvider: kubeProvider},
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

// udtSecretsLoader reads cleartext for Radius.Security/secrets resources from their backing Kubernetes Secret.
type udtSecretsLoader struct {
	databaseClient database.Client
	kubeProvider   *kubernetesclientprovider.KubernetesClientProvider
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

// loadSecret resolves a single Radius.Security/secrets resource to its cleartext data. It fails closed: if the
// backing Kubernetes Secret cannot be located or read, an error is returned rather than partial/empty data.
func (l *udtSecretsLoader) loadSecret(ctx context.Context, secretID string, keysFilter []string) (recipes.SecretData, error) {
	if l.databaseClient == nil || l.kubeProvider == nil {
		return recipes.SecretData{}, fmt.Errorf("secret loader is not fully configured for Radius.Security/secrets")
	}

	resource, err := database.GetResource[datamodel.DynamicResource](ctx, l.databaseClient, secretID)
	if err != nil {
		return recipes.SecretData{}, fmt.Errorf("failed to get secret resource %q: %w", secretID, err)
	}

	namespace, name, found := kubernetesSecretLocation(resource.OutputResources())
	if !found {
		return recipes.SecretData{}, fmt.Errorf("secret %q has no Kubernetes Secret output resource; only Kubernetes-backed Radius.Security/secrets are supported", secretID)
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
