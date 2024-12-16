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
	"fmt"
	"sync"

	"github.com/radius-project/radius/pkg/ucp/store"
)

// DataStorageProvider acts as a factory for storage clients.
//
// Do not use construct this directly:
//
// - Use DataStorageProviderFromOptions instead for production use.
// - Use DataStorageProviderFromMemory or DataStorageProviderFromClient for testing.
type DataStorageProvider struct {
	// options configures the settings of the storage provider.
	options StorageProviderOptions

	// factory is factory function to create a new storage client. Can be overridden for testing.
	factory storageFactoryFunc

	// init is used to guarantee single-initialization of the storage provider.
	init sync.RWMutex

	// result is result of invoking the factory (cached).
	result result
}

type result struct {
	client store.StorageClient
	err    error
}

// DataStorageProviderFromOptions creates a new instance of the DataStorageProvider struct with the given options.
//
// This will used the known factory functions to instantiate the storage client.
func DataStorageProviderFromOptions(options StorageProviderOptions) *DataStorageProvider {
	return &DataStorageProvider{options: options}
}

// DataStorageProviderFromMemory creates a new instance of the DataStorageProvider struct using the in-memory client.
//
// This will use the ephemeral in-memory storage client.
func DataStorageProviderFromMemory() *DataStorageProvider {
	return &DataStorageProvider{options: StorageProviderOptions{Provider: TypeInMemory}}
}

// DataStorageProviderFromClient creates a new instance of the DataStorageProvider struct with the given client.
//
// This will always return the given client and will not attempt to create a new one. This can be used for testing
// with mocks.
func DataStorageProviderFromClient(client store.StorageClient) *DataStorageProvider {
	return &DataStorageProvider{result: result{client: client}}
}

// GetStorageClient returns a storage client for the given resource type.
func (p *DataStorageProvider) GetClient(ctx context.Context) (store.StorageClient, error) {
	// Guarantee single initialization.
	p.init.RLock()
	result := p.result
	p.init.RUnlock()

	if result.client == nil && result.err == nil {
		result = p.initialize(ctx)
	}

	// Invariant, either result.err is set or result.client is set.
	if result.err != nil {
		return nil, result.err
	}

	if result.client == nil {
		panic("invariant violated: p.result.client is nil")
	}

	return result.client, nil
}

func (p *DataStorageProvider) initialize(ctx context.Context) result {
	p.init.Lock()
	defer p.init.Unlock()

	// Invariant: p.result is set when this function exits.
	// Invariant: p.result.client is nil or p.result.err is nil when this function exits.
	// Invariant: p.result is returned to the caller, so they don't need to retake the lock.

	// Note: this is a double-checked locking pattern.
	//
	// It's possible that result was set by another goroutine before we acquired the lock.
	if p.result.client != nil || p.result.err != nil {
		return p.result
	}

	// If we get here we have the exclusive lock and need to initialize the storage client.

	factory := p.factory
	if factory == nil {
		fn, ok := storageClientFactory[p.options.Provider]
		if !ok {
			p.result = result{nil, fmt.Errorf("unsupported storage provider: %s", p.options.Provider)}
			return p.result
		}

		factory = fn
	}

	client, err := factory(ctx, p.options)
	if err != nil {
		p.result = result{nil, fmt.Errorf("failed to initialize storage client: %w", err)}
		return p.result
	} else if client == nil {
		p.result = result{nil, fmt.Errorf("failed to initialize storage client: provider returned nil")}
		return p.result
	}

	p.result = result{client, nil}
	return p.result
}
