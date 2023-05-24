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
