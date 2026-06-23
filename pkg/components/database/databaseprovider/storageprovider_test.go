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

func Test_GetClient_CachedError(t *testing.T) {
	provider := FromOptions(Options{Provider: "Test"})

	expectedErr := errors.New("oh noes!")

	callCount := 0
	provider.factory = databaseClientFactoryFunc(func(ctx context.Context, options Options) (database.Client, error) {
		callCount++
		return nil, expectedErr
	})

	client, err := provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: oh noes!", err.Error())
	require.Nil(t, client)

	// Do it twice to ensure the client is cached.
	client, err = provider.GetClient(context.Background())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: oh noes!", err.Error())
	require.Nil(t, client)

	require.Equal(t, 1, callCount)
}

func TestGetClient_UnsupportedProvider(t *testing.T) {
	options := Options{Provider: "unsupported"}
	provider := FromOptions(options)

	client, err := provider.GetClient(context.Background())

	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported database provider: unsupported", err.Error())
}

func Test_expandEnvURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		env      map[string]string
		expected string
	}{
		{
			name:     "no substitution",
			url:      "postgresql://user:pass@host:5432/db",
			expected: "postgresql://user:pass@host:5432/db",
		},
		{
			name:     "single variable is expanded in place",
			url:      "postgresql://ucp:${POSTGRES_PASSWORD}@database:5432/ucp",
			env:      map[string]string{"POSTGRES_PASSWORD": "s3cret"},
			expected: "postgresql://ucp:s3cret@database:5432/ucp",
		},
		{
			name: "multiple variables are all expanded",
			url:  "postgresql://${PGUSER}:${PGPASS}@host:5432/db",
			env: map[string]string{
				"PGUSER": "ucp",
				"PGPASS": "p@ss",
			},
			expected: "postgresql://ucp:p@ss@host:5432/db",
		},
		{
			name:     "unset variable expands to empty string",
			url:      "postgresql://ucp:${MISSING}@host:5432/db",
			expected: "postgresql://ucp:@host:5432/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			require.Equal(t, tt.expected, expandEnvURL(tt.url))
		})
	}
}

func TestInitialize(t *testing.T) {
	options := Options{Provider: TypeInMemory}
	provider := FromOptions(options)

	result := provider.initialize(context.Background())

	require.NoError(t, result.err)
	require.NotNil(t, result.client)
}
