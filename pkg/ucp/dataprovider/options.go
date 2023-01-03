// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"github.com/project-radius/radius/pkg/ucp/hosting"
	etcdclient "go.etcd.io/etcd/client/v3"
)

// StorageProviderOptions represents the data storage provider options.
type StorageProviderOptions struct {
	// Provider configures the storage provider.
	Provider StorageProviderType `yaml:"provider"`

	// APIServer configures options for the Kubernetes APIServer store. Will be ignored if another store is configured.
	APIServer APIServerOptions `yaml:"apiserver,omitempty"`

	// CosmosDB configures options for the CosmosDB store. Will be ignored if another store is configured.
	CosmosDB CosmosDBOptions `yaml:"cosmosdb,omitempty"`

	// ETCD configures options for the etcd store. Will be ignored if another store is configured.
	ETCD ETCDOptions `yaml:"etcd,omitempty"`
}

// APIServerOptions represents options for the configuring the Kubernetes APIServer store.
type APIServerOptions struct {
	// InCluster configures the APIServer store to use "in-cluster" credentials. Use this when running inside a Kubernetes cluster.
	InCluster bool `yaml:"incluster"`

	// Context configures the Kubernetes context name to use for the connection. Use this for NON-production scenarios to test
	// against a specific cluster.
	Context string `yaml:"context"`

	// Namespace configures the Kubernetes namespace used for data-storage. The namespace must already exist.
	Namespace string `yaml:"namespace"`
}

// CosmosDBOptions represents cosmosdb options for data storage provider.
type CosmosDBOptions struct {
	Url                  string `yaml:"url"`
	Database             string `yaml:"database"`
	MasterKey            string `yaml:"masterKey"`
	CollectionThroughput int    `yaml:"collectionThroughput,omitempty"`
}

// ETCDOptions represents options for the configuring the etcd store.
type ETCDOptions struct {
	// InMemory configures the etcd store to run in-memory with the resource provider. This is not suitable for production use.
	InMemory bool `yaml:"inmemory"`

	// Client is used to access the etcd client when running in memory.
	//
	// NOTE: when we run etcd in memory it will be registered as its own hosting.Service with its own startup/shutdown lifecyle.
	// We need a way to share state between the etcd service and the things that want to consume it. This is that.
	Client *hosting.AsyncValue[etcdclient.Client] `yaml:"-"`
}
