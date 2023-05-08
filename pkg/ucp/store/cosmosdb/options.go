/*
------------------------------------------------------------
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
------------------------------------------------------------
*/

package cosmosdb

import "github.com/project-radius/radius/pkg/ucp/store"

const (
	defaultQueryItemCount = 20
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
