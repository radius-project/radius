// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package provider

// QueueProviderOptions represents the data storage provider options.
type QueueProviderOptions struct {
	// Provider configures the storage provider.
	Provider QueueProviderType `yaml:"provider"`

	InMemory InMemoryQueueOptions `yaml:"inMemoryQueue"`
}

// InMemoryQueueOptions represents the inmemory queue options.
type InMemoryQueueOptions struct {
	// Name is the name of inmemory queue.
	Name string
}
