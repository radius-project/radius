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
}
