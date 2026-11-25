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

// Options represents the database provider options.
type Options struct {
	// Provider configures the database provider.
	Provider DatabaseProviderType `yaml:"provider"`

	// APIServer configures options for the Kubernetes APIServer store. Will be ignored if another store is configured.
	APIServer APIServerOptions `yaml:"apiserver,omitempty"`

	// InMemory configures options for the in-memory store. Will be ignored if another store is configured.
	InMemory InMemoryOptions `yaml:"inmemory,omitempty"`

	// PostgreSQL configures options for connecting to a PostgreSQL database. Will be ignored if another store is configured.
	PostgreSQL PostgreSQLOptions `yaml:"postgresql,omitempty"`
}

// APIServerOptions represents options for the configuring the Kubernetes APIServer store.
type APIServerOptions struct {
	// Context configures the Kubernetes context name to use for the connection. Use this for NON-production scenarios to test
	// against a specific cluster.
	Context string `yaml:"context"`

	// Namespace configures the Kubernetes namespace used for data-storage. The namespace must already exist.
	Namespace string `yaml:"namespace"`
}

// InMemoryOptions represents options for the in-memory store.
type InMemoryOptions struct{}

// PostgreSQLOptions represents options for the PostgreSQL store.
type PostgreSQLOptions struct {
	// URL is the connection information for the PostgreSQL database in URL format.
	//
	// The URL should be formatted according to:
	// https://www.postgresql.org/docs/current/libpq-connect.html#LIBPQ-CONNSTRING-URIS
	//
	// The URL can contain secrets like passwords so it must be treated as sensitive.
	//
	// In place of the actual URL, you can substitute an environment variable by using the format:
	// 	${ENV_VAR_NAME}
	URL string `yaml:"url"`

	// AuthMode selects how the client authenticates to the server. Defaults to PostgreSQLAuthPassword.
	AuthMode PostgreSQLAuthMode `yaml:"authMode,omitempty"`

	// Azure configures Microsoft Entra authentication. Used only when AuthMode is PostgreSQLAuthAzure.
	Azure PostgreSQLAzureOptions `yaml:"azure,omitempty"`
}

// PostgreSQLAuthMode is the authentication method for the PostgreSQL store.
type PostgreSQLAuthMode string

const (
	// PostgreSQLAuthPassword authenticates with the password in the connection URL.
	PostgreSQLAuthPassword PostgreSQLAuthMode = "password"

	// PostgreSQLAuthAzure authenticates to Azure Database for PostgreSQL with a Microsoft Entra access
	// token, requested for each new connection. The URL user must be the PostgreSQL role mapped to the
	// identity, the URL must not contain a password, and sslmode must be verify-full so the token is only
	// sent to a verified server. Only the Azure public cloud is supported.
	PostgreSQLAuthAzure PostgreSQLAuthMode = "azure"
)

// PostgreSQLAzureCredential is the Azure identity used for Microsoft Entra authentication.
type PostgreSQLAzureCredential string

const (
	// PostgreSQLAzureWorkloadIdentity uses Azure workload identity. The tenant ID and federated token file
	// come from AZURE_TENANT_ID and AZURE_FEDERATED_TOKEN_FILE, which the AKS workload identity webhook sets.
	PostgreSQLAzureWorkloadIdentity PostgreSQLAzureCredential = "workloadIdentity"

	// PostgreSQLAzureManagedIdentity uses the managed identity of the host, such as a virtual machine.
	PostgreSQLAzureManagedIdentity PostgreSQLAzureCredential = "managedIdentity"
)

// PostgreSQLAzureOptions represents options for Microsoft Entra authentication to PostgreSQL.
type PostgreSQLAzureOptions struct {
	// Credential selects the Azure identity. Defaults to PostgreSQLAzureWorkloadIdentity.
	Credential PostgreSQLAzureCredential `yaml:"credential,omitempty"`

	// ClientID is the client ID of the identity. For workload identity it defaults to AZURE_CLIENT_ID.
	// For a managed identity it is required for a user-assigned identity and empty for a system-assigned one.
	ClientID string `yaml:"clientId,omitempty"`
}
