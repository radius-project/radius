// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

// SecretsProviderType represents types of secrets provider.
type SecretsProviderType string

const (
	// TypeAPIServer represents the Kubernetes APIServer provider.
	TypeETCDSecrets SecretsProviderType = "etcd"

	// TypeCosmosDB represents CosmosDB provider.
	TypeKubernetesSecrets SecretsProviderType = "kubernetes"
)