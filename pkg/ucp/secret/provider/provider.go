// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/project-radius/radius/pkg/ucp/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/secret"
)

var (
	ErrUnsupportedSecretProvider = errors.New("unsupported secrets provider")
	ErrSecretNotFound            = errors.New("secrets provider not found")
)

var _ SecretProvider = (*secretProvider)(nil)

type secretProvider struct {
	secretClient secret.Client
	options      SecretProviderOptions
	once         sync.Once
}

// NewSecretProvider creates new SecretsStorageProvider instance.
func NewSecretProvider(opts SecretProviderOptions) SecretProvider {
	return &secretProvider{
		secretClient: nil,
		options:      opts,
	}
}

// GetSecretClient creates or gets secrets interface.
func (p *secretProvider) GetSecretClient(ctx context.Context, secretsType string) (secret.Client, error) {
	if p.secretClient != nil {
		return p.secretClient, nil
	}

	err := provider.ErrUnsupportedStorageProvider
	p.once.Do(func() {
		if fn, ok := secretClientFactory[p.options.Provider]; ok {
			p.secretClient, err = fn(ctx, p.options)
		}
	})

	return p.secretClient, err
}
