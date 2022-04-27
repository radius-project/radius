// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

// StorageProviderOptions represents the data storage provider options.
type StorageProviderOptions struct {
	Provider StorageProviderType `yaml:"provider"`
	CosmosDB CosmosDBOptions     `yaml:"cosmosdb,omitempty"`
}

// CosmosDBOptions represents cosmosdb options for data storage provider.
type CosmosDBOptions struct {
	Url                  string `yaml:"url"`
	Database             string `yaml:"database"`
	MasterKey            string `yaml:"masterKey"`
	CollectionThroughput int    `yaml:"collectionThroughput,omitempty"`
}
