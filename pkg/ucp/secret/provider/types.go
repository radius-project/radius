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
	// TypeETCDSecrets represents the ETCD secret provider.
	TypeETCDSecrets SecretProviderType = "etcd"

	// TypeKubernetesSecrets represents the Kubernetes provider.
	TypeKubernetesSecrets SecretProviderType = "kubernetes"
)

//go:generate mockgen -destination=./mock_secretprovider.go -package=provider -self_package github.com/project-radius/radius/pkg/ucp/secret/provider github.com/project-radius/radius/pkg/ucp/secret/provider SecretProvider

// SecretProvider is an interface to provide secrets storage interface.
type SecretProvider interface {
	// GetSecretClient returns the secret client if it has been initialized already, if not, creates it and then returns it.
	GetSecretClient(context.Context) (secret.Client, error)
}
