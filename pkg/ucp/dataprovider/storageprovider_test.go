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
	"testing"

	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/stretchr/testify/require"
)

func Test_DataStorageProviderFromOptions(t *testing.T) {
	options := StorageProviderOptions{Provider: TypeInMemory}
	provider := DataStorageProviderFromOptions(options)

	require.NotNil(t, provider)
	require.Equal(t, options, provider.options)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_DataStorageProviderFromMemory(t *testing.T) {
	provider := DataStorageProviderFromMemory()

	require.NotNil(t, provider)
	require.Equal(t, TypeInMemory, provider.options.Provider)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_DataStorageProviderFromClient(t *testing.T) {
	mockClient := &store.MockStorageClient{}
	provider := DataStorageProviderFromClient(mockClient)

	require.NotNil(t, provider)
	require.Same(t, mockClient, provider.result.client)

	client, err := provider.GetClient(context.Background())
	require.NoError(t, err)
	require.Same(t, client, mockClient)
}

func Test_GetClient_CachedClient(t *testing.T) {
	mockClient := &store.MockStorageClient{}
	provider := DataStorageProviderFromOptions(StorageProviderOptions{Provider: "Test"})

	callCount := 0
	provider.factory = storageFactoryFunc(func(ctx context.Context, options StorageProviderOptions) (store.StorageClient, error) {
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

func Test_GetClient_CachedError(t *testing.T) {
	provider := DataStorageProviderFromOptions(StorageProviderOptions{Provider: "Test"})

	expectedErr := errors.New("oh noes!")

	callCount := 0
	provider.factory = storageFactoryFunc(func(ctx context.Context, options StorageProviderOptions) (store.StorageClient, error) {
		callCount++
		return nil, expectedErr
	})

	client, err := provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize storage client: oh noes!", err.Error())
	require.Nil(t, client)

	// Do it twice to ensure the client is cached.
	client, err = provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize storage client: oh noes!", err.Error())
	require.Nil(t, client)

	require.Equal(t, 1, callCount)
}

func TestGetClient_UnsupportedProvider(t *testing.T) {
	options := StorageProviderOptions{Provider: "unsupported"}
	provider := DataStorageProviderFromOptions(options)

	client, err := provider.GetClient(context.Background())

	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported storage provider: unsupported", err.Error())
}

func TestInitialize(t *testing.T) {
	options := StorageProviderOptions{Provider: TypeInMemory}
	provider := DataStorageProviderFromOptions(options)

	result := provider.initialize(context.Background())

	require.NoError(t, result.err)
	require.NotNil(t, result.client)
}
