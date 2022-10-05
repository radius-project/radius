// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package secretsprovider

import (
	"context"
	"errors"
	"sync"

	"github.com/project-radius/radius/pkg/ucp/secrets"
	"github.com/project-radius/radius/pkg/ucp/util"
)

var (
	ErrUnsupportedSecretsProvider = errors.New("unsupported secrets provider")
	ErrSecretsNotFound            = errors.New("secrets provider not found")
)

var _ SecretsStorageProvider = (*secretsProvider)(nil)

type secretsProvider struct {
	clients   map[string]secrets.Interface
	clientsMu sync.RWMutex
	options   SecretsProviderOptions
}

// NewSecretsProvider creates new SecretsStorageProvider instance.
func NewSecretsProvider(opts SecretsProviderOptions) SecretsStorageProvider {
	return &secretsProvider{
		clients: map[string]secrets.Interface{},
		options: opts,
	}
}

// GetSecretsInterface creates or gets secrets interface.
func (p *secretsProvider) GetSecretsInterface(ctx context.Context, resourceType string) (secrets.Interface, error) {
	cn := util.NormalizeStringToLower(resourceType)

	p.clientsMu.RLock()
	c, ok := p.clients[cn]
	p.clientsMu.RUnlock()
	if ok {
		return c, nil
	}

	var err error
	if fn, ok := secretsClientFactory[p.options.Provider]; ok {
		// This write lock ensure that storage init function executes one by one and write client
		// to map safely.
		// CosmosDBStorageClient Init() calls database and collection creation control plane APIs.
		// Ideally, such control plane APIs must be idempotent, but we could see unexpected failures
		// by calling control plane API concurrently. Even if such issue rarely happens during release
		// time, it could make the short-term downtime of the service.
		// We expect that GetStorageClient() will be called during the start time. Thus, having a lock won't
		// hurt any runtime performance.
		p.clientsMu.Lock()
		defer p.clientsMu.Unlock()

		if c, ok := p.clients[cn]; ok {
			return c, nil
		}

		if c, err = fn(ctx, p.options); err == nil {
			p.clients[cn] = c
		}
	} else {
		err = ErrUnsupportedSecretsProvider
	}

	return c, err
}
