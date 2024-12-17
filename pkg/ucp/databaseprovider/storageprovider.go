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

package databaseprovider

import (
	"context"
	"fmt"
	"sync"

	"github.com/radius-project/radius/pkg/ucp/database"
)

// DatabaseProvider acts as a factory for database clients.
//
// Do not use construct this directly:
//
// - Use FromOptions instead for production use.
// - Use FromMemory or FromClient for testing.
type DatabaseProvider struct {
	// options configures the settings of the database provider.
	options Options

	// factory is factory function to create a new database client. Can be overridden for testing.
	factory databaseClientFactoryFunc

	// init is used to guarantee single-initialization of the database provider.
	init sync.RWMutex

	// result is result of invoking the factory (cached).
	result result
}

type result struct {
	client database.Client
	err    error
}

// FromOptions creates a new instance of the DatabaseProvider struct with the given options.
//
// This will used the known factory functions to instantiate the database client.
func FromOptions(options Options) *DatabaseProvider {
	return &DatabaseProvider{options: options}
}

// FromMemory creates a new instance of the DatabaseProvider struct using the in-memory client.
//
// This will use the ephemeral in-memory database client.
func FromMemory() *DatabaseProvider {
	return &DatabaseProvider{options: Options{Provider: TypeInMemory}}
}

// FromClient creates a new instance of the DatabaseProvider struct with the given client.
//
// This will always return the given client and will not attempt to create a new one. This can be used for testing
// with mocks.
func FromClient(client database.Client) *DatabaseProvider {
	return &DatabaseProvider{result: result{client: client}}
}

// GetClient returns a database client for the given resource type.
func (p *DatabaseProvider) GetClient(ctx context.Context) (database.Client, error) {
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

func (p *DatabaseProvider) initialize(ctx context.Context) result {
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

	// If we get here we have the exclusive lock and need to initialize the database client.

	factory := p.factory
	if factory == nil {
		fn, ok := databaseClientFactory[p.options.Provider]
		if !ok {
			p.result = result{nil, fmt.Errorf("unsupported database provider: %s", p.options.Provider)}
			return p.result
		}

		factory = fn
	}

	client, err := factory(ctx, p.options)
	if err != nil {
		p.result = result{nil, fmt.Errorf("failed to initialize database client: %w", err)}
		return p.result
	} else if client == nil {
		p.result = result{nil, fmt.Errorf("failed to initialize database client: provider returned nil")}
		return p.result
	}

	p.result = result{client, nil}
	return p.result
}
