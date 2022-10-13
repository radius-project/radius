// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/secret"
)

// SecretProviderType represents types of secrets provider.
type SecretProviderType string

const (
	// TypeAPIServer represents the Kubernetes APIServer provider.
	TypeETCDSecrets SecretProviderType = "etcd"

	// TypeCosmosDB represents CosmosDB provider.
	TypeKubernetesSecrets SecretProviderType = "kubernetes"
)

//go:generate mockgen -destination=./mock_secretprovider.go -package=provider -self_package github.com/project-radius/radius/pkg/ucp/secret/provider github.com/project-radius/radius/pkg/ucp/secret/provider SecretProvider

// SecretProvider is an interfae to provide secrets storage interface.
type SecretProvider interface {
	// GetSecretClient creates or gets secrets interface.
	GetSecretClient(context.Context, string) (secret.Client, error)
}
