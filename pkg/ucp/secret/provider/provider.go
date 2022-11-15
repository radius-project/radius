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

type secretProvider struct {
	client  secret.Client
	options SecretProviderOptions
	once    sync.Once
}

// NewSecretProvider creates new SecretProvider instance.
func NewSecretProvider(opts SecretProviderOptions) *secretProvider {
	return &secretProvider{
		client:  nil,
		options: opts,
	}
}

// GetSecretClient returns the secret client if it has been initialized already, if not, creates it and then returns it.
func (p *secretProvider) GetClient(ctx context.Context) (secret.Client, error) {
	if p.client != nil {
		return p.client, nil
	}

	err := ErrUnsupportedSecretProvider
	p.once.Do(func() {
		if fn, ok := secretClientFactory[p.options.Provider]; ok {
			p.client, err = fn(ctx, p.options)
		}
	})

	return p.client, err
}
