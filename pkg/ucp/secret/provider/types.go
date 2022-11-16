// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// SecretProviderType represents types of secret provider.
type SecretProviderType string

const (
	// TypeETCDSecret represents the ETCD secret provider.
	TypeETCDSecret SecretProviderType = "etcd"

	// TypeKubernetesSecret represents the Kubernetes secret provider.
	TypeKubernetesSecret SecretProviderType = "kubernetes"
)
