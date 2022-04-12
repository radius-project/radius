// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package store

import (
	"context"

	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

type CosmosDBClient struct {
	client *cosmosapi.Client
}

var _ StorageClient = (*CosmosDBClient)(nil)

func NewCosmosDBClient() *CosmosDBClient {
	return &CosmosDBClient{
		client: nil,
	}
}

func (c *CosmosDBClient) Init() {
	c.client = cosmosapi.New()

}

func (c *CosmosDBClient) CreateContainer(containerName string) error {
	return nil
}

func (c *CosmosDBClient) Query(ctx context.Context, query Query, options ...QueryOptions) ([]Object, error) {
	return nil, nil
}

func (c *CosmosDBClient) Get(ctx context.Context, id string, options ...GetOptions) (*Object, error) {
	return nil, nil
}

func (c *CosmosDBClient) Delete(ctx context.Context, id string, options ...DeleteOptions) error {
	return nil
}

func (c *CosmosDBClient) Save(ctx context.Context, obj *Object, options ...SaveOptions) error {
	return nil
}
