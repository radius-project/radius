// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"context"
	"errors"
	"strings"
	"sync"
	"unicode"

	"github.com/project-radius/radius/pkg/store"
)

var (
	ErrUnsupportedStorageProvider = errors.New("unsupported storage provider")
	ErrStorageNotFound            = errors.New("storage provider not found")
)

var _ DataStorageProvider = (*storageProvider)(nil)

type storageProvider struct {
	clients   map[string]store.StorageClient
	clientsMu sync.RWMutex
	options   StorageProviderOptions
}

// NewStorageProvider creates new DataStorageProvider instance.
func NewStorageProvider(opts StorageProviderOptions) DataStorageProvider {
	return &storageProvider{
		clients: map[string]store.StorageClient{},
		options: opts,
	}
}

// GetStorageClient creates or gets storage client.
func (p *storageProvider) GetStorageClient(ctx context.Context, resourceType string) (store.StorageClient, error) {
	cn := normalizeResourceType(resourceType)

	p.clientsMu.RLock()
	c, ok := p.clients[cn]
	p.clientsMu.RUnlock()
	if ok {
		return c, nil
	}

	var err error
	if fn, ok := storageClientFactory[p.options.Provider]; ok {
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

		if c, err = fn(ctx, p.options, cn); err == nil {
			p.clients[cn] = c
		}
	} else {
		err = ErrUnsupportedStorageProvider
	}

	return c, err
}

// normalizeResourceType converts resourcetype to safe string by removing non digit and non letter and replace '/' with '-'
func normalizeResourceType(s string) string {
	if s == "" {
		return s
	}

	sb := strings.Builder{}
	for _, ch := range s {
		if ch == '/' {
			sb.WriteString("-")
		} else if unicode.IsDigit(ch) || unicode.IsLetter(ch) {
			sb.WriteRune(ch)
		}
	}

	return strings.ToLower(sb.String())
}
