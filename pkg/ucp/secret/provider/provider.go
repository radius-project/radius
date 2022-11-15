// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

import (
	"context"
	"errors"
	"sync"

	"github.com/project-radius/radius/pkg/ucp/secret"
)

var (
	ErrUnsupportedSecretProvider = errors.New("unsupported secret provider")
	ErrSecretNotFound            = errors.New("secret not found")
)

var _ SecretProvider = (*secretProvider)(nil)

type secretProvider struct {
	secretClient secret.Client
	options      SecretProviderOptions
	once         sync.Once
}

// NewSecretProvider creates new SecretProvider instance.
func NewSecretProvider(opts SecretProviderOptions) SecretProvider {
	return &secretProvider{
		secretClient: nil,
		options:      opts,
	}
}

// GetSecretClient returns the secret client if it has been initialized already, if not, creates it and then returns it.
func (p *secretProvider) GetSecretClient(ctx context.Context) (secret.Client, error) {
	if p.secretClient != nil {
		return p.secretClient, nil
	}

	err := ErrUnsupportedSecretProvider
	p.once.Do(func() {
		if fn, ok := secretClientFactory[p.options.Provider]; ok {
			p.secretClient, err = fn(ctx, p.options)
		}
	})

	return p.secretClient, err
}
