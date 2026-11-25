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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/jackc/pgx/v5"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/stretchr/testify/require"
)

func Test_FromOptions(t *testing.T) {
	options := Options{Provider: TypeInMemory}
	provider := FromOptions(options)

	require.NotNil(t, provider)
	require.Equal(t, options, provider.options)

	client, err := provider.GetClient(t.Context())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_FromMemory(t *testing.T) {
	provider := FromMemory()

	require.NotNil(t, provider)
	require.Equal(t, TypeInMemory, provider.options.Provider)

	client, err := provider.GetClient(t.Context())
	require.NoError(t, err)
	require.NotNil(t, client)
}

func Test_FromClient(t *testing.T) {
	mockClient := &database.MockClient{}
	provider := FromClient(mockClient)

	require.NotNil(t, provider)
	require.Same(t, mockClient, provider.result.client)

	client, err := provider.GetClient(t.Context())
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

	client, err := provider.GetClient(t.Context())
	require.NoError(t, err)
	require.Same(t, mockClient, client)

	// Do it twice to ensure the client is cached.
	client, err = provider.GetClient(t.Context())
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

	client, err := provider.GetClient(t.Context())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: oh noes!", err.Error())
	require.Nil(t, client)

	// Do it twice to ensure the client is cached.
	client, err = provider.GetClient(t.Context())
	require.Error(t, err)
	require.Equal(t, "failed to initialize database client: oh noes!", err.Error())
	require.Nil(t, client)

	require.Equal(t, 1, callCount)
}

func TestGetClient_UnsupportedProvider(t *testing.T) {
	options := Options{Provider: "unsupported"}
	provider := FromOptions(options)

	client, err := provider.GetClient(t.Context())

	require.Error(t, err)
	require.Nil(t, client)
	require.Equal(t, "unsupported database provider: unsupported", err.Error())
}

func Test_expandEnvURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		env       map[string]string
		expected  string
		expectErr bool
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
			name:     "empty-but-set variable expands to empty string",
			url:      "postgresql://ucp:${EMPTY_VAR}@host:5432/db",
			env:      map[string]string{"EMPTY_VAR": ""},
			expected: "postgresql://ucp:@host:5432/db",
		},
		{
			name:      "unset variable fails fast",
			url:       "postgresql://ucp:${DEFINITELY_NOT_SET_VAR}@host:5432/db",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := expandEnvURL(tt.url)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func TestInitialize(t *testing.T) {
	options := Options{Provider: TypeInMemory}
	provider := FromOptions(options)

	result := provider.initialize(t.Context())

	require.NoError(t, result.err)
	require.NotNil(t, result.client)
}

type fakeTokenCredential struct {
	token  string
	err    error
	scopes []string
}

func (f *fakeTokenCredential) GetToken(_ context.Context, options policy.TokenRequestOptions) (azcore.AccessToken, error) {
	f.scopes = options.Scopes
	return azcore.AccessToken{Token: f.token}, f.err
}

// isolatePostgresEnv keeps libpq environment defaults from changing the parsed password or TLS settings.
func isolatePostgresEnv(t *testing.T) {
	for _, name := range []string{"PGPASSWORD", "PGSERVICE", "PGSERVICEFILE", "PGSSLMODE", "PGSSLROOTCERT", "PGSSLCERT", "PGSSLKEY"} {
		t.Setenv(name, "")
	}
	t.Setenv("PGPASSFILE", filepath.Join(t.TempDir(), "missing"))
}

func Test_newPostgreSQLPoolConfig(t *testing.T) {
	isolatePostgresEnv(t)

	const verifiedURL = "postgresql://radius-identity@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full"

	tests := []struct {
		name        string
		options     PostgreSQLOptions
		pgPassword  string
		passfile    string
		serviceFile string
		credErr     error
		expectErr   string
		expectToken bool
	}{
		{
			name:    "password mode is the default and keeps the URL password",
			options: PostgreSQLOptions{URL: "postgresql://ucp:s3cret@localhost:5432/ucp?sslmode=disable"},
		},
		{
			name:    "explicit password mode",
			options: PostgreSQLOptions{URL: "postgresql://ucp:s3cret@localhost:5432/ucp", AuthMode: PostgreSQLAuthPassword},
		},
		{
			name:        "azure mode sets a token for each connection",
			options:     PostgreSQLOptions{URL: verifiedURL, AuthMode: PostgreSQLAuthAzure},
			expectToken: true,
		},
		{
			name:      "azure mode rejects a password in the URL",
			options:   PostgreSQLOptions{URL: "postgresql://ucp:s3cret@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full", AuthMode: PostgreSQLAuthAzure},
			expectErr: "authMode azure does not use a password",
		},
		{
			name:        "azure mode accepts system roots, which pgx verifies like verify-full",
			options:     PostgreSQLOptions{URL: "postgresql://radius-identity@example.postgres.database.azure.com:5432/ucp?sslmode=require&sslrootcert=system", AuthMode: PostgreSQLAuthAzure},
			expectToken: true,
		},
		{
			name:      "azure mode rejects an empty password in the URL",
			options:   PostgreSQLOptions{URL: "postgresql://ucp:@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the URL",
		},
		{
			name:      "azure mode rejects a password query parameter",
			options:   PostgreSQLOptions{URL: "postgresql://ucp@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full&password=", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the URL",
		},
		{
			name:      "azure mode rejects an empty password in a multi-host URL",
			options:   PostgreSQLOptions{URL: "postgresql://ucp:@host1.postgres.database.azure.com:5432,host2.postgres.database.azure.com/ucp?sslmode=verify-full", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the URL",
		},
		{
			name:      "azure mode rejects a password keyword",
			options:   PostgreSQLOptions{URL: "host=example.postgres.database.azure.com user=ucp password=s3cret sslmode=verify-full", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the connection string",
		},
		{
			name:      "azure mode rejects an empty quoted password keyword",
			options:   PostgreSQLOptions{URL: "host=example.postgres.database.azure.com user=ucp sslmode=verify-full password=''", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the connection string",
		},
		{
			name:      "azure mode rejects an empty trailing password keyword",
			options:   PostgreSQLOptions{URL: "host=example.postgres.database.azure.com user=ucp sslmode=verify-full password=", AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the connection string",
		},
		{
			name:      "azure mode rejects an empty password keyword after a value with an escaped quote",
			options:   PostgreSQLOptions{URL: `host=example.postgres.database.azure.com application_name='it\'s' password='' user=ucp sslmode=verify-full`, AuthMode: PostgreSQLAuthAzure},
			expectErr: "remove it from the connection string",
		},
		{
			name:        "azure mode accepts password text after an escaped quote inside another keyword value",
			options:     PostgreSQLOptions{URL: `host=example.postgres.database.azure.com user=radius-identity dbname=ucp sslmode=verify-full application_name='it\'s password='`, AuthMode: PostgreSQLAuthAzure},
			expectToken: true,
		},
		{
			name:        "azure mode accepts escaped password text in an unquoted keyword value",
			options:     PostgreSQLOptions{URL: `host=example.postgres.database.azure.com user=radius-identity dbname=ucp sslmode=verify-full application_name=a\ password=`, AuthMode: PostgreSQLAuthAzure},
			expectToken: true,
		},
		{
			name:        "azure mode accepts password text inside another keyword value",
			options:     PostgreSQLOptions{URL: "host=example.postgres.database.azure.com user=radius-identity dbname=ucp sslmode=verify-full application_name='my password=none'", AuthMode: PostgreSQLAuthAzure},
			expectToken: true,
		},
		{
			name:        "azure mode rejects a password from a service file",
			options:     PostgreSQLOptions{URL: "postgresql://radius-identity@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full&service=radius", AuthMode: PostgreSQLAuthAzure},
			serviceFile: "[radius]\npassword=s3cret\n",
			expectErr:   "PGSERVICEFILE",
		},
		{
			name:       "azure mode rejects PGPASSWORD",
			options:    PostgreSQLOptions{URL: verifiedURL, AuthMode: PostgreSQLAuthAzure},
			pgPassword: "s3cret",
			expectErr:  "unset PGPASSWORD",
		},
		{
			name:      "azure mode rejects a matching password file entry",
			options:   PostgreSQLOptions{URL: verifiedURL, AuthMode: PostgreSQLAuthAzure},
			passfile:  "example.postgres.database.azure.com:5432:ucp:radius-identity:s3cret\n",
			expectErr: "PGPASSFILE",
		},
		{
			name:      "azure mode rejects sslmode=require",
			options:   PostgreSQLOptions{URL: "postgresql://ucp@example.postgres.database.azure.com:5432/ucp?sslmode=require", AuthMode: PostgreSQLAuthAzure},
			expectErr: "requires TLS with certificate and hostname verification",
		},
		{
			name:      "azure mode rejects sslmode=verify-ca",
			options:   PostgreSQLOptions{URL: "postgresql://ucp@example.postgres.database.azure.com:5432/ucp?sslmode=verify-ca", AuthMode: PostgreSQLAuthAzure},
			expectErr: "requires TLS with certificate and hostname verification",
		},
		{
			name:      "azure mode rejects a plaintext fallback",
			options:   PostgreSQLOptions{URL: "postgresql://ucp@example.postgres.database.azure.com:5432/ucp?sslmode=prefer", AuthMode: PostgreSQLAuthAzure},
			expectErr: "requires TLS with certificate and hostname verification",
		},
		{
			name:      "azure mode reports credential errors",
			options:   PostgreSQLOptions{URL: verifiedURL, AuthMode: PostgreSQLAuthAzure},
			credErr:   errors.New("no identity"),
			expectErr: "failed to create Azure credential: no identity",
		},
		{
			name:      "azure options without authMode azure",
			options:   PostgreSQLOptions{URL: verifiedURL, Azure: PostgreSQLAzureOptions{Credential: PostgreSQLAzureManagedIdentity}},
			expectErr: "azure options require authMode azure",
		},
		{
			name:      "unsupported auth mode",
			options:   PostgreSQLOptions{URL: verifiedURL, AuthMode: "kerberos"},
			expectErr: `unsupported authMode "kerberos"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.pgPassword != "" {
				t.Setenv("PGPASSWORD", tt.pgPassword)
			}
			if tt.passfile != "" {
				passfile := filepath.Join(t.TempDir(), "pgpass")
				require.NoError(t, os.WriteFile(passfile, []byte(tt.passfile), 0o600))
				t.Setenv("PGPASSFILE", passfile)
			}

			if tt.serviceFile != "" {
				serviceFile := filepath.Join(t.TempDir(), "pg_service.conf")
				require.NoError(t, os.WriteFile(serviceFile, []byte(tt.serviceFile), 0o600))
				t.Setenv("PGSERVICEFILE", serviceFile)
			}

			credential := &fakeTokenCredential{token: "entra-token"}
			var gotAzure *PostgreSQLAzureOptions
			newCredential := func(opt PostgreSQLAzureOptions) (azcore.TokenCredential, error) {
				gotAzure = &opt
				return credential, tt.credErr
			}

			config, err := newPostgreSQLPoolConfig(tt.options, newCredential)

			if tt.expectErr != "" {
				require.ErrorContains(t, err, tt.expectErr)
				require.Nil(t, config)
				return
			}
			require.NoError(t, err)
			if !tt.expectToken {
				require.Nil(t, config.BeforeConnect)
				require.Nil(t, gotAzure)
				require.Equal(t, "s3cret", config.ConnConfig.Password)
				return
			}
			require.Equal(t, &tt.options.Azure, gotAzure)
			require.NotNil(t, config.BeforeConnect)
			connConfig := config.ConnConfig.Copy()
			require.NoError(t, config.BeforeConnect(t.Context(), connConfig))
			require.Equal(t, "entra-token", connConfig.Password)
			require.Equal(t, []string{azurePostgreSQLScope}, credential.scopes)
		})
	}
}

func Test_azureTokenBeforeConnect_Error(t *testing.T) {
	credential := &fakeTokenCredential{err: errors.New("token request failed")}
	config := &pgx.ConnConfig{}

	err := azureTokenBeforeConnect(credential, time.Minute)(t.Context(), config)

	require.ErrorContains(t, err, "token request failed")
	require.Empty(t, config.Password)
}

type deadlineTokenCredential struct {
	deadline time.Time
}

func (d *deadlineTokenCredential) GetToken(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	d.deadline, _ = ctx.Deadline()
	return azcore.AccessToken{Token: "entra-token"}, nil
}

func Test_newPostgreSQLPoolConfig_TokenTimeout(t *testing.T) {
	isolatePostgresEnv(t)

	tests := []struct {
		name   string
		url    string
		expect time.Duration
	}{
		{
			name:   "connect_timeout bounds the token request",
			url:    "postgresql://radius-identity@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full&connect_timeout=5",
			expect: 5 * time.Second,
		},
		{
			name:   "default when connect_timeout is unset",
			url:    "postgresql://radius-identity@example.postgres.database.azure.com:5432/ucp?sslmode=verify-full",
			expect: defaultAzureTokenTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential := &deadlineTokenCredential{}
			config, err := newPostgreSQLPoolConfig(PostgreSQLOptions{URL: tt.url, AuthMode: PostgreSQLAuthAzure}, func(PostgreSQLAzureOptions) (azcore.TokenCredential, error) {
				return credential, nil
			})
			require.NoError(t, err)

			// pgxpool sets a 2 minute ConnectTimeout on the copy it passes to BeforeConnect when none is configured.
			connConfig := config.ConnConfig.Copy()
			connConfig.ConnectTimeout = 2 * time.Minute
			start := time.Now()
			require.NoError(t, config.BeforeConnect(t.Context(), connConfig))
			require.WithinDuration(t, start.Add(tt.expect), credential.deadline, time.Second)
		})
	}
}

func Test_newAzureTokenCredential(t *testing.T) {
	t.Run("managed identity", func(t *testing.T) {
		// Clear the variables the SDK uses to detect a managed identity source, so the test uses IMDS.
		for _, name := range []string{"IDENTITY_ENDPOINT", "IDENTITY_HEADER", "IDENTITY_SERVER_THUMBPRINT", "IMDS_ENDPOINT", "MSI_ENDPOINT", "MSI_SECRET"} {
			t.Setenv(name, "")
		}
		credential, err := newAzureTokenCredential(PostgreSQLAzureOptions{Credential: PostgreSQLAzureManagedIdentity, ClientID: "00000000-0000-0000-0000-000000000001"})
		require.NoError(t, err)
		require.IsType(t, &azidentity.ManagedIdentityCredential{}, credential)
	})

	t.Run("workload identity is the default", func(t *testing.T) {
		t.Setenv("AZURE_TENANT_ID", "00000000-0000-0000-0000-000000000002")
		t.Setenv("AZURE_FEDERATED_TOKEN_FILE", filepath.Join(t.TempDir(), "token"))
		credential, err := newAzureTokenCredential(PostgreSQLAzureOptions{ClientID: "00000000-0000-0000-0000-000000000001"})
		require.NoError(t, err)
		require.IsType(t, &azidentity.WorkloadIdentityCredential{}, credential)
	})

	t.Run("unsupported credential", func(t *testing.T) {
		_, err := newAzureTokenCredential(PostgreSQLAzureOptions{Credential: "clientSecret"})
		require.EqualError(t, err, `unsupported azure credential "clientSecret"`)
	})
}
