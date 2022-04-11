// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/corerp/store"
	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

const (
	PartitionKeyName = "/partitionKey"
)

type CosmosDBStorageClient struct {
	client  *cosmosapi.Client
	options *CosmosDBClientOptions
}

type CosmosDBClientOptions struct {
	Url                string
	MasterKeyAuthCreds string
	DatabaseName       string
	CollectionName     string
}

type CosmosDBEntity struct {
	ID           string      `json:"id"`
	ResourceID   string      `json:"resourceId"`
	PartitionKey string      `json:"partitionKey"`
	UpdatedTime  time.Time   `json:"updatedTime"`
	Entity       interface{} `json:"entity"`

	ETag      string `json:"_etag"`
	Self      string `json:"_self"`
	Timestamp int    `json:"_ts"`
}

var _ store.StorageClient = (*CosmosDBStorageClient)(nil)

func NewCosmosDBClient(options *CosmosDBClientOptions) (*CosmosDBStorageClient, error) {
	// TODO: Create the custom client transport to support service principal and managed identity.
	cfg := cosmosapi.Config{
		MasterKey:  options.MasterKeyAuthCreds,
		MaxRetries: 5,
	}

	clientTransport, err := NewAzureIdentityTransport("")
	if err != nil {
		return nil, err
	}

	httpClient := &http.Client{
		Transport: clientTransport,
	}

	client := cosmosapi.New(options.Url, cfg, httpClient, nil)

	return &CosmosDBStorageClient{
		client:  client,
		options: options,
	}, nil
}

func (c *CosmosDBStorageClient) Init() error {
	if err := c.createDatabaseIfNotExists(); err != nil {
		return err
	}
	if err := c.createCollectionIfNotExists(); err != nil {
		return err
	}
	return nil
}

func (c *CosmosDBStorageClient) createDatabaseIfNotExists() error {
	_, err := c.client.GetDatabase(context.Background(), c.options.DatabaseName, nil)
	if err == nil {
		return nil
	}
	if err != nil && err.Error() != "Resource that no longer exists" {
		return errors.WithStack(err)
	}

	_, err = c.client.CreateDatabase(context.Background(), c.options.DatabaseName, nil)
	return err
}

func (c *CosmosDBStorageClient) createCollectionIfNotExists() error {
	_, err := c.client.GetCollection(context.Background(), c.options.DatabaseName, c.options.CollectionName)
	if err == nil {
		return nil
	}
	if err != nil && err.Error() != "Resource that no longer exists" {
		return errors.WithStack(err)
	}
	_, err = c.client.CreateCollection(context.Background(), c.options.DatabaseName, cosmosapi.CreateCollectionOptions{
		Id: c.options.CollectionName,
		IndexingPolicy: &cosmosapi.IndexingPolicy{
			IndexingMode: cosmosapi.IndexingMode("consistent"),
			Automatic:    true,
			Included: []cosmosapi.IncludedPath{
				{
					Path: "/*",
					Indexes: []cosmosapi.Index{
						{
							Kind:      cosmosapi.Range,
							DataType:  cosmosapi.StringType,
							Precision: -1,
						},
						{
							Kind:      cosmosapi.Range,
							DataType:  cosmosapi.NumberType,
							Precision: -1,
						},
					},
				},
			},
		},
		PartitionKey: &cosmosapi.PartitionKey{
			Paths: []string{
				PartitionKeyName,
			},
			Kind: "Hash",
		},
		OfferThroughput: 4000,
	})

	return err
}

func (c *CosmosDBStorageClient) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) ([]store.Object, error) {
	// WIP
	return nil, nil
}

func (c *CosmosDBStorageClient) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
	azID, err := azresources.Parse(id)
	if err != nil {
		return nil, err
	}

	ops := cosmosapi.GetDocumentOptions{
		PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
	}

	docID := GenerateResourceID(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	entity := &CosmosDBEntity{}
	_, err = c.client.GetDocument(context.Background(), c.options.DatabaseName, c.options.CollectionName, docID, ops, entity)

	obj := &store.Object{
		Metadata: store.Metadata{
			ID:   entity.ResourceID,
			ETag: entity.ETag,
		},
		Data: entity.Entity,
	}

	return obj, err
}

func (c *CosmosDBStorageClient) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
	azID, err := azresources.Parse(id)
	if err != nil {
		return err
	}

	ops := cosmosapi.DeleteDocumentOptions{
		PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
	}
	_, err = c.client.DeleteDocument(ctx, c.options.DatabaseName, c.options.CollectionName, id, ops)
	return errors.WithStack(err)
}

func (c *CosmosDBStorageClient) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) error {
	azID, err := azresources.Parse(obj.ID)
	if err != nil {
		return err
	}

	entity := CosmosDBEntity{
		ID:           GenerateResourceID(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name()),
		ResourceID:   obj.ID,
		PartitionKey: NormalizeSubscriptionID(azID.SubscriptionID),
		UpdatedTime:  time.Now().UTC(),
		Entity:       obj.Data,
	}

	if obj.ETag == "" {
		op := cosmosapi.CreateDocumentOptions{
			PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
			IsUpsert:          true,
		}
		_, _, err = c.client.CreateDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity, op)
	} else {
		op := cosmosapi.ReplaceDocumentOptions{
			PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
			IfMatch:           obj.ETag,
		}
		_, _, err = c.client.ReplaceDocument(context.Background(), c.options.DatabaseName, c.options.CollectionName, obj.ID, entity, op)
	}

	return nil
}
