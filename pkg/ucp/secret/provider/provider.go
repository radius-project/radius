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

// SecretProvider creates client based on the options provided.
type SecretProvider struct {
	client  secret.Client
	options SecretProviderOptions
	once    sync.Once
}

// NewSecretProvider creates a new SecretProvider instance with the given options.
func NewSecretProvider(opts SecretProviderOptions) *SecretProvider {
	return &SecretProvider{
		client:  nil,
		options: opts,
	}
}

// SetClient sets the secret client for the SecretProvider. This should be used by tests that need to mock the secret client.
func (p *SecretProvider) SetClient(client secret.Client) {
	p.client = client
}

// GetClient checks if a secret client has already been created, and if not, creates one using the secretClientFactory
// map. If the provider is not supported, an error is returned.
func (p *SecretProvider) GetClient(ctx context.Context) (secret.Client, error) {
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
