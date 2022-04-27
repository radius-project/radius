// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import "github.com/project-radius/radius/pkg/store"

const (
	defaultQueryItemCount       = 20
	defaultCollectionThroughPut = 400
)

// ConnectionOptions represents connection info to connect CosmosDB
type ConnectionOptions struct {
	// Url represents the url of cosmosdb endpoint.
	Url string
	// DatabaseName represents the database name to connect.
	DatabaseName string
	// CollectionName represents the collection name in DataBaseName
	CollectionName string
	// DefaultQueryItemCount represents the maximum number of items for query.
	DefaultQueryItemCount int
	// CollectionThroughput represents shared throughput database share the throughput (RU/s) allocated to that database.
	CollectionThroughput int

	// MasterKey is the key string for CosmosDB connection.
	MasterKey string
}

func (c *ConnectionOptions) load() error {
	if c.MasterKey == "" {
		return &store.ErrInvalid{Message: "unset MasterKey"}
	}

	if c.DefaultQueryItemCount == 0 {
		c.DefaultQueryItemCount = defaultQueryItemCount
	}

	return nil
}
