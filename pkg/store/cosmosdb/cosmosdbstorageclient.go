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

const (
	// PartitionKeyName is the property used for partitioning.
	PartitionKeyName = "/partitionKey"

	// go-cosmosdb does not return the error response code. Comparing error message is the only way to check the errors.
	// Once we move to official Go SDK, we can have the better error handling.
	// TODO: Switch to the official cosmosdb SDK - https://github.com/project-radius/radius/issues/2225
	// 1. Repalce github.com/vippsas/go-cosmosdb/cosmosapi with the official sdk when it supports query api.
	// 2. Improve error handling using response code instead of string match.
	errResourceNotFoundMsg       = "Resource that no longer exists"
	errIDConflictMsg             = "The ID provided has been taken by an existing resource"
	errEtagPreconditionMsgPrefix = "The operation specified an eTag"
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
	if err != nil && !strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return err
	}
	_, err = c.client.CreateDatabase(ctx, c.options.DatabaseName, nil)
	if err != nil && strings.EqualFold(err.Error(), errIDConflictMsg) {
		return nil
	}
	return err
}

func (c *CosmosDBStorageClient) createCollectionIfNotExists(ctx context.Context) error {
	_, err := c.client.GetCollection(ctx, c.options.DatabaseName, c.options.CollectionName)
	if err == nil {
		return nil
	}
	if err != nil && !strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return err
	}
	opt := cosmosapi.CreateCollectionOptions{
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
	}

	// CollectionThroughput must be set only if radius uses Provioned throughput mode.
	if c.options.CollectionThroughput > 0 {
		opt.OfferThroughput = cosmosapi.OfferThroughput(c.options.CollectionThroughput)
	}

	_, err = c.client.CreateCollection(context.Background(), c.options.DatabaseName, opt)

	if err != nil && strings.EqualFold(err.Error(), errIDConflictMsg) {
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

	// TODO: Support ScopeRecursive and RoutingScopePrefix later for UCP - https://github.com/project-radius/radius/issues/2224
	if query.ScopeRecursive || query.RoutingScopePrefix != "" {
		return nil, nil, &store.ErrInvalid{Message: "ScopeRecursive and RoutingScopePrefix are not supported."}
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

	maxItemCount := c.options.DefaultQueryItemCount
	if cfg.MaxQueryItemCount > 0 {
		maxItemCount = cfg.MaxQueryItemCount
	}

	qops := cosmosapi.QueryDocumentsOptions{
		IsQuery:              true,
		ContentType:          cosmosapi.QUERY_CONTENT_TYPE,
		MaxItemCount:         maxItemCount,
		EnableCrossPartition: true,
		ConsistencyLevel:     cosmosapi.ConsistencyLevelEventual,
	}

	// if subscriptionid is given, then use partition key.
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

	if err != nil && strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
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
func (c *CosmosDBStorageClient) Delete(ctx context.Context, id string, opts ...store.DeleteOptions) error {
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
	if err != nil && strings.EqualFold(err.Error(), errResourceNotFoundMsg) {
		return &store.ErrNotFound{}
	}

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

	partitionKey := NormalizeSubscriptionID(azID.SubscriptionID)

	var resp *cosmosapi.Resource
	if ifMatch == "" {
		// Use CreateDocument to create new document or upsert document if etag is not given
		op := cosmosapi.CreateDocumentOptions{
			PartitionKeyValue: partitionKey,
			IsUpsert:          true,
		}
		resp, _, err = c.client.CreateDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity, op)
	} else {
		// Use ReplaceDocument to update doc if etag is given.
		op := cosmosapi.ReplaceDocumentOptions{
			PartitionKeyValue: partitionKey,
			IfMatch:           ifMatch,
		}
		resp, _, err = c.client.ReplaceDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity.ID, entity, op)
		// TODO: use the response code when switching to official SDK.
		if err != nil && strings.HasPrefix(err.Error(), errEtagPreconditionMsgPrefix) {
			return nil, &store.ErrConflict{Message: "ETag is not matched."}
		}
	}

	if err != nil {
		return nil, err
	}

	obj.ETag = resp.Etag

	return obj, nil
}
