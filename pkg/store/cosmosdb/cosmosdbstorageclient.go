// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/store"
	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

// TODO: When the official cosmosdb SDK supports query api, https://github.com/Azure/azure-sdk-for-go/tree/sdk/data/azcosmos/v0.2.0/sdk/data/azcosmos/
// 1. Repalce github.com/vippsas/go-cosmosdb/cosmosapi with the official one
// 2. Improve error handling using response code instead of string match.

const (
	// PartitionKeyName is the property used for partitioning.
	PartitionKeyName     = "/partitionKey"
	collectionThroughPut = 4000

	// go-cosmosdb does not return the error response code. Comparing error message is the only way to check the errors.
	// Once we move to official Go SDK, we can have the better error handling.
	errResourceNotFoundMsg = "Resource that no longer exists"
	errIDConflictMsg       = "The ID provided has been taken by an existing resource"
)

var _ store.StorageClient = (*CosmosDBStorageClient)(nil)

// ResourceEntity represents the default envelope model to store resource metadata.
type ResourceEntity struct {
	// CosmosDB system-related properties.
	// ID represents the primary key.
	ID string `json:"id"`
	// ETag represents an etag required for optimistic concurrency control.
	ETag string `json:"_etag"`
	// Self represents the unique addressable URI for the resource.
	Self string `json:"_self"`
	// Timestamp represents the last updated timestamp of the resource.
	UpdatedTime int `json:"_ts"`

	// ResourceID represents fully qualified resource id.
	ResourceID string `json:"resourceId"`
	// RootScope represents root scope such as subscription id.
	RootScope string `json:"rootScope"`
	// ResourceGroup represents fully qualified resource scope.
	ResourceGroup string `json:"resourceGroup"`
	// PartitionKey represents the key used for partitioning.
	PartitionKey string `json:"partitionKey"`
	// Entity represents the resource metadata.
	Entity interface{} `json:"entity"`
}

// CosmosDBStorageClient implements CosmosDB stroage client.
type CosmosDBStorageClient struct {
	client  *cosmosapi.Client
	options *ConnectionOptions
}

// NewCosmosDBStorageClient creates a new CosmosDBStorageClient.
func NewCosmosDBStorageClient(options *ConnectionOptions) (*CosmosDBStorageClient, error) {
	if err := options.load(); err != nil {
		return nil, err
	}

	cfg := cosmosapi.Config{
		MasterKey:  options.MasterKey,
		MaxRetries: 5,
	}

	client := cosmosapi.New(options.Url, cfg, nil, nil)

	return &CosmosDBStorageClient{
		client:  client,
		options: options,
	}, nil
}

// Init initializes the database and collection.
func (c *CosmosDBStorageClient) Init(ctx context.Context) error {
	if err := c.createDatabaseIfNotExists(ctx); err != nil {
		return err
	}
	if err := c.createCollectionIfNotExists(ctx); err != nil {
		return err
	}
	return nil
}

func (c *CosmosDBStorageClient) createDatabaseIfNotExists(ctx context.Context) error {
	_, err := c.client.GetDatabase(ctx, c.options.DatabaseName, nil)
	if err == nil {
		return nil
	}
	// TODO: Switch to the better error handling once we switch to official cosmosdb sdk.
	if err != nil && !strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return err
	}
	_, err = c.client.CreateDatabase(ctx, c.options.DatabaseName, nil)
	// TODO: Switch to the better error handling once we switch to official cosmosdb sdk.
	if strings.EqualFold(err.Error(), errIDConflictMsg) {
		return nil
	}
	return err
}

func (c *CosmosDBStorageClient) createCollectionIfNotExists(ctx context.Context) error {
	_, err := c.client.GetCollection(ctx, c.options.DatabaseName, c.options.CollectionName)
	if err == nil {
		return nil
	}
	// TODO: Switch to the better error handling once we switch to official cosmosdb sdk.
	if err != nil && !strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return err
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
		OfferThroughput: collectionThroughPut,
	})

	// TODO: Switch to the better error handling once we switch to official cosmosdb sdk.
	if strings.EqualFold(err.Error(), errIDConflictMsg) {
		return nil
	}

	return err
}

func constructCosmosDBQuery(query store.Query) (*ResourceScope, *cosmosapi.Query, error) {
	// Validate query request.
	resourceScope, err := NewResourceScope(query.RootScope)
	if err != nil {
		return nil, nil, err
	}

	// TODO: Support RoutingScopePrefix later when we need.
	if query.RoutingScopePrefix != "" {
		return nil, nil, &store.ErrInvalid{Message: "RoutingScopePrefix is not supported"}
	}

	// Construct SQL query
	queryString := "SELECT * FROM c WHERE "
	whereParam := ""
	queryParams := []cosmosapi.QueryParam{}
	if resourceScope.SubscriptionID != "" {
		whereParam = whereParam + "c.rootScope = @rootScope"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rootScope",
			Value: resourceScope.fullyQualifiedSubscriptionScope(),
		})
	}

	if resourceScope.ResourceGroup != "" {
		if whereParam != "" {
			whereParam += " and "
		}
		whereParam += "c.resourceGroup = @rgName"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rgName",
			Value: resourceScope.ResourceGroup,
		})
	}

	if query.ResourceType != "" {
		if whereParam != "" {
			whereParam += " and "
		}
		whereParam += "STRINGEQUALS(c.entity.type, @rtype, true)"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rtype",
			Value: query.ResourceType,
		})
	}

	for i, filter := range query.Filters {
		if whereParam != "" {
			whereParam += " and "
		}
		filterParam := fmt.Sprintf("filter%d", i)
		whereParam += fmt.Sprintf("STRINGEQUALS(c.entity.%s, @%s, true)", filter.Field, filterParam)
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@" + filterParam,
			Value: filter.Value,
		})
	}

	if whereParam == "" {
		return nil, nil, &store.ErrInvalid{Message: "invalid Query parameters"}
	}

	return resourceScope, &cosmosapi.Query{Query: queryString + whereParam, Params: queryParams}, nil
}

// Query queries the data resource
func (c *CosmosDBStorageClient) Query(ctx context.Context, query store.Query, opts ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	cfg := store.NewQueryConfig(opts...)

	// Prepare document query.
	resourceScope, qry, err := constructCosmosDBQuery(query)
	if err != nil {
		return nil, err
	}

	entities := []ResourceEntity{}

	qops := cosmosapi.QueryDocumentsOptions{
		IsQuery:              true,
		ContentType:          cosmosapi.QUERY_CONTENT_TYPE,
		MaxItemCount:         c.options.MaxQueryItemCount,
		EnableCrossPartition: true,
		ConsistencyLevel:     cosmosapi.ConsistencyLevelEventual,
	}

	if resourceScope.SubscriptionID != "" {
		qops.PartitionKeyValue = NormalizeSubscriptionID(resourceScope.SubscriptionID)
		qops.EnableCrossPartition = false
	}

	if cfg.PaginationToken != "" {
		qops.Continuation = cfg.PaginationToken
	}

	resp, err := c.client.QueryDocuments(ctx, c.options.DatabaseName, c.options.CollectionName, *qry, &entities, qops)
	if err != nil {
		return nil, err
	}

	// Prepare response
	output := []store.Object{}
	for _, entity := range entities {
		output = append(output, store.Object{
			Metadata: store.Metadata{
				ID:   entity.ResourceID,
				ETag: entity.ETag,
			},
			Data: entity.Entity,
		})
	}

	return &store.ObjectQueryResult{
		PaginationToken: resp.Continuation,
		Items:           output,
	}, nil
}

// Get gets the resource data using id.
func (c *CosmosDBStorageClient) Get(ctx context.Context, id string, opts ...store.GetOptions) (*store.Object, error) {
	azID, err := azresources.Parse(id)
	if err != nil {
		return nil, err
	}

	ops := cosmosapi.GetDocumentOptions{
		PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
	}

	docID, err := GenerateCosmosDBKey(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	if err != nil {
		return nil, err
	}
	entity := &ResourceEntity{}
	_, err = c.client.GetDocument(ctx, c.options.DatabaseName, c.options.CollectionName, docID, ops, entity)

	if strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return nil, &store.ErrNotFound{}
	}

	obj := &store.Object{
		Metadata: store.Metadata{
			ID:   entity.ResourceID,
			ETag: entity.ETag,
		},
		Data: entity.Entity,
	}

	return obj, err
}

// Delete deletes the resource using id.
func (c *CosmosDBStorageClient) Delete(ctx context.Context, id string, options ...store.DeleteOptions) error {
	azID, err := azresources.Parse(id)
	if err != nil {
		return err
	}

	ops := cosmosapi.DeleteDocumentOptions{
		PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
	}
	docID, err := GenerateCosmosDBKey(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	if err != nil {
		return err
	}
	_, err = c.client.DeleteDocument(ctx, c.options.DatabaseName, c.options.CollectionName, docID, ops)

	return err
}

// Save upserts the resource.
func (c *CosmosDBStorageClient) Save(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) (*store.Object, error) {
	cfg := store.NewSaveConfig(opts...)
	azID, err := azresources.Parse(obj.ID)
	if err != nil {
		return nil, err
	}

	docID, err := GenerateCosmosDBKey(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	if err != nil {
		return nil, err
	}

	rs, err := NewResourceScope(azID.ID)
	if err != nil {
		return nil, err
	}

	entity := &ResourceEntity{
		ID:            docID,
		ResourceID:    azID.ID,
		RootScope:     rs.fullyQualifiedSubscriptionScope(),
		ResourceGroup: rs.ResourceGroup,
		PartitionKey:  NormalizeSubscriptionID(azID.SubscriptionID),
		Entity:        obj.Data,
	}

	ifMatch := cfg.ETag
	if ifMatch == "" && obj.ETag != "" {
		ifMatch = obj.ETag
	}

	var resp *cosmosapi.Resource
	if obj.ETag == "" {
		op := cosmosapi.CreateDocumentOptions{
			PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
			IsUpsert:          true,
		}
		resp, _, err = c.client.CreateDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity, op)
	} else {
		op := cosmosapi.ReplaceDocumentOptions{
			PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
			IfMatch:           ifMatch,
		}
		resp, _, err = c.client.ReplaceDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity.ID, entity, op)
	}

	if err != nil {
		return nil, &store.ErrInvalid{Message: err.Error()}
	}

	obj.ETag = resp.Etag

	return obj, nil
}
