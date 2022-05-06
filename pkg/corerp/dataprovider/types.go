// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	"context"

	"github.com/project-radius/radius/pkg/store"
)

// StorageProviderType represents types of storage provider.
type StorageProviderType string

const (
	// CosmosDBProvider represents CosmosDB provider.
	CosmosDBProvider StorageProviderType = "cosmosdb"

	// InMemoryProvider is an in-memory ephemeral store useful for testing.
	InMemoryProvider StorageProviderType = "memory"
)

//go:generate mockgen -destination=./mock_datastorage_provider.go -package=dataprovider -self_package github.com/project-radius/radius/pkg/corerp/dataprovider github.com/project-radius/radius/pkg/corerp/dataprovider DataStorageProvider

// DataStorageProvider is an interfae to provide storage client.
type DataStorageProvider interface {
	// GetStorageClient creates or gets storage client.
	GetStorageClient(context.Context, string) (store.StorageClient, error)
}
