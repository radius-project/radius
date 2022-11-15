// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"

	"github.com/project-radius/radius/pkg/ucp/secret"
)

// SecretProviderType represents types of secret provider.
type SecretProviderType string

const (
	// TypeETCDSecret represents the ETCD secret provider.
	TypeETCDSecret SecretProviderType = "etcd"

	// TypeKubernetesSecret represents the Kubernetes provider.
	TypeKubernetesSecret SecretProviderType = "kubernetes"
)

//go:generate mockgen -destination=./mock_secretprovider.go -package=provider -self_package github.com/project-radius/radius/pkg/ucp/secret/provider github.com/project-radius/radius/pkg/ucp/secret/provider SecretProvider

// SecretProvider is an interface to provide secret interface.
type SecretProvider interface {
	// GetSecretClient returns the secret client if it has been initialized already, if not, creates it and then returns it.
	GetSecretClient(context.Context) (secret.Client, error)
}
