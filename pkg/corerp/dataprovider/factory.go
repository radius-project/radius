// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package dataprovider

import (
	context "context"
	"fmt"

	store "github.com/project-radius/radius/pkg/store"
	"github.com/project-radius/radius/pkg/store/cosmosdb"
)

type storageFactoryFunc func(context.Context, StorageProviderOptions, string) (store.StorageClient, error)

var storageClientFactory = map[StorageProviderType]storageFactoryFunc{
	CosmosDBProvider: initCosmosDBClient,
}

func initCosmosDBClient(ctx context.Context, opt StorageProviderOptions, collectionName string) (store.StorageClient, error) {
	sopt := &cosmosdb.ConnectionOptions{
		Url:                  opt.CosmosDB.Url,
		DatabaseName:         opt.CosmosDB.Database,
		CollectionName:       collectionName,
		MasterKey:            opt.CosmosDB.MasterKey,
		CollectionThroughput: opt.CosmosDB.CollectionThroughput,
	}
	dbclient, err := cosmosdb.NewCosmosDBStorageClient(sopt)
	if err != nil {
		return nil, fmt.Errorf("failed to create CosmosDB client - configuration may be invalid: %w", err)
	}

	if err = dbclient.Init(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize CosmosDB client - configuration may be invalid: %w", err)
	}

	return dbclient, nil
}
