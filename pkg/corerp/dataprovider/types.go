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
)

// DataStorageProvider is an interfae to provide storage client.
type DataStorageProvider interface {
	// GetStorageClient creates or gets storage client.
	GetStorageClient(context.Context, string) (store.StorageClient, error)
}
