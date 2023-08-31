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

package dataprovider

import (
	"context"
	"errors"
	"sync"

	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/pkg/ucp/util"
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

// NewStorageProvider creates a new instance of the "storageProvider" struct with the given
// "StorageProviderOptions" and returns it.
func NewStorageProvider(opts StorageProviderOptions) DataStorageProvider {
	return &storageProvider{
		clients: map[string]store.StorageClient{},
		options: opts,
	}
}

// GetStorageClient checks if a StorageClient for the given resourceType already exists in the map, and
// if so, returns it. If not, it creates a new StorageClient using the storageClientFactory and adds it to the map,
// returning it. If an error occurs, it returns an error.
func (p *storageProvider) GetStorageClient(ctx context.Context, resourceType string) (store.StorageClient, error) {
	cn := util.NormalizeStringToLower(resourceType)

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
