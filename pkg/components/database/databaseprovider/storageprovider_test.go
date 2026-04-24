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
	"errors"
	"testing"

	"github.com/radius-project/radius/pkg/components/database"
	"github.com/stretchr/testify/require"
)

func Test_FromOptions(t *testing.T) {
	options := Options{Provider: TypeInMemory}
	provider := FromOptions(options)

	require.NotNil(t, provider)
	require.Equal(t, options, provider.options)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_FromMemory(t *testing.T) {
	provider := FromMemory()

	require.NotNil(t, provider)
	require.Equal(t, TypeInMemory, provider.options.Provider)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_FromClient(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromClient(mockClient)

	require.NotNil(t, provider)
	require.Same(t, mockClient, provider.result.client)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, client, mockClient)
}

func Test_GetClient_CachedClient(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromOptions(Options{Provider: "Test"})

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		return mockClient, nil
	})

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, mockClient, client)

	// Do it twice to ensure the client is cached.
	client, err = provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, mockClient, client)

	require.Equal(t, 1, callCount)
}

func Test_GetClient_RetryAfterError(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromOptions(Options{Provider: "Test"})

	expectedErr := errors.New("oh noes!")

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		if callCount == 1 {
			return nil, expectedErr
		}
		return mockClient, nil
	})

	// First call should fail
	client, err := provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: oh noes!", err.Error())
	require.Nil(t, client)

	// Second call should retry and succeed
	client, err = provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, mockClient, client)

	require.Equal(t, 2, callCount)

	// Third call should use cached successful result
	client, err = provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, mockClient, client)

	require.Equal(t, 2, callCount)
}

func Test_GetClient_FactoryReturnsNil(t *testing.T) {
	provider := FromOptions(Options{Provider: "Test"})

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		return nil, nil
	})

	// First call should fail because factory returned nil
	client, err := provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: provider returned nil", err.Error())
	require.Nil(t, client)

	// Error should NOT be cached - factory should be called again
	client, err = provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: provider returned nil", err.Error())
	require.Nil(t, client)

	require.Equal(t, 2, callCount)
}

func TestGetClient_UnsupportedProvider(t *testing.T) {
	options := Options{Provider: "unsupported"}
	provider := FromOptions(options)

	client, err := provider.GetClient(context.Background())

	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported database provider: unsupported", err.Error())
}

func TestGetClient_UnsupportedProvider_Cached(t *testing.T) {
	provider := FromOptions(Options{Provider: "unsupported"})

	// First call
	client, err := provider.GetClient(context.Background())
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported database provider: unsupported", err.Error())

	// Second call should return same cached error (unsupported provider is a config error, not transient)
	client, err = provider.GetClient(context.Background())
	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported database provider: unsupported", err.Error())
}

func TestSetFactory(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromOptions(Options{Provider: "Test"})

	provider.SetFactory(func(ctx context.Context, options Options) (database.Client, error) {
		return mockClient, nil
	})

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, mockClient, client)
}

func TestGetClient_ConcurrentAccess(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromOptions(Options{Provider: "Test"})

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		return mockClient, nil
	})

	// Launch multiple goroutines to access GetClient concurrently
	const numGoroutines = 10
	results := make(chan database.Client, numGoroutines)
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			client, err := provider.GetClient(context.Background())
			results <- client
			errs <- err
		}()
	}

	// Verify all goroutines got the same client
	for i := 0; i < numGoroutines; i++ {
		client := <-results
		err := <-errs
		require.NoError(t, err)
		require.Same(t, mockClient, client)
	}

	// Factory should have been called exactly once
	require.Equal(t, 1, callCount)
}

func TestInitialize(t *testing.T) {
	options := Options{Provider: TypeInMemory}
	provider := FromOptions(options)

	result := provider.initialize(context.Background())

	require.NoError(t, result.err)
	require.NotNil(t, result.client)
}

func TestInitialize_DoubleCheckedLocking(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromOptions(Options{Provider: "Test"})

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		return mockClient, nil
	})

	// First initialize call
	result := provider.initialize(context.Background())
	require.NoError(t, result.err)
	require.Same(t, mockClient, result.client)

	// Second initialize call should return cached result without calling factory
	result = provider.initialize(context.Background())
	require.NoError(t, result.err)
	require.Same(t, mockClient, result.client)

	require.Equal(t, 1, callCount)
}
