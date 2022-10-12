// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/secrets"
)

// SecretsProviderType represents types of secrets provider.
type SecretsProviderType string

const (
	// TypeAPIServer represents the Kubernetes APIServer provider.
	TypeETCDSecrets SecretsProviderType = "etcd"

	// TypeCosmosDB represents CosmosDB provider.
	TypeKubernetesSecrets SecretsProviderType = "kubernetes"
)

//go:generate mockgen -destination=./mock_secretsstorage_provider.go -package=secretsprovider -self_package github.com/project-radius/radius/pkg/ucp/secretsprovider github.com/project-radius/radius/pkg/ucp/secretsprovider SecretsStorageProvider

// SecretsProvider is an interfae to provide secrets storage interface.
type SecretsProvider interface {
	// GetSecretsInterface creates or gets secrets interface.
	GetSecretsInterface(context.Context, string) (secrets.Interface, error)
}
