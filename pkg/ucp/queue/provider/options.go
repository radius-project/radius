// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// QueueProviderOptions represents the data storage provider options.
type QueueProviderOptions struct {
	// Provider configures the storage provider.
	Provider QueueProviderType `yaml:"provider"`

	// InMemory represents inmemory queue client options. (Optional)
	InMemory *InMemoryQueueOptions `yaml:"inMemoryQueue,omitempty"`

	// APIServer configures options for the Kubernetes APIServer store. (Optional)
	APIServer APIServerOptions `yaml:"apiserver,omitempty"`
}

// InMemoryQueueOptions represents the inmemory queue options.
type InMemoryQueueOptions struct {
}

// APIServerOptions represents options for the configuring the Kubernetes APIServer store.
type APIServerOptions struct {
	// Context configures the Kubernetes context name to use for the connection. Use this for NON-production scenarios to test
	// against a specific cluster.
	Context string `yaml:"context"`

	// Namespace configures the Kubernetes namespace used for data-storage. The namespace must already exist.
	Namespace string `yaml:"namespace"`
}
