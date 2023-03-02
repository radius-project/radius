// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"strings"

	"github.com/project-radius/radius/pkg/ucp/resources"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
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
	Entity any `json:"entity"`
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

	// CollectionThroughput needs to be set only if radius uses Provioned throughput mode.
	if c.options.CollectionThroughput > 0 {
		opt.OfferThroughput = cosmosapi.OfferThroughput(c.options.CollectionThroughput)
	}

	_, err = c.client.CreateCollection(context.Background(), c.options.DatabaseName, opt)

	if err != nil && strings.EqualFold(err.Error(), errIDConflictMsg) {
		return nil
	}

	return err
}

func constructCosmosDBQuery(query store.Query) (*cosmosapi.Query, error) {
	if query.RoutingScopePrefix != "" {
		return nil, &store.ErrInvalid{Message: "RoutingScopePrefix is not supported."}
	}

	if query.RootScope == "" {
		return nil, &store.ErrInvalid{Message: "RootScope can not be empty."}
	}

	queryString := "SELECT * FROM c WHERE "
	whereParam := ""
	queryParams := []cosmosapi.QueryParam{}

	if query.ScopeRecursive {
		whereParam = whereParam + "STARTSWITH(c.rootScope, @rootScope, true)"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rootScope",
			Value: strings.ToLower(query.RootScope),
		})
	} else {
		whereParam = whereParam + "c.rootScope = @rootScope"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rootScope",
			Value: strings.ToLower(query.RootScope),
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
		return nil, &store.ErrInvalid{Message: "invalid Query parameters"}
	}

	return &cosmosapi.Query{Query: queryString + whereParam, Params: queryParams}, nil
}

// Query queries the data resource
func (c *CosmosDBStorageClient) Query(ctx context.Context, query store.Query, opts ...store.QueryOptions) (*store.ObjectQueryResult, error) {
	if ctx == nil {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if query.RootScope == "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RootScope' is required"}
	}
	if query.IsScopeQuery && query.RoutingScopePrefix != "" {
		return nil, &store.ErrInvalid{Message: "invalid argument. 'query.RoutingScopePrefix' is not supported for scope queries"}
	}

	cfg := store.NewQueryConfig(opts...)

	resourceID, err := resources.ParseScope(query.RootScope)
	if err != nil {
		return nil, err
	}

	qry, err := constructCosmosDBQuery(query)
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

	partitionKey, err := GetPartitionKey(resourceID)
	if err != nil {
		return nil, err
	}

	if partitionKey != "" {
		qops.PartitionKeyValue = partitionKey
		qops.EnableCrossPartition = false
	}

	if cfg.PaginationToken != "" {
		qops.Continuation = cfg.PaginationToken
	}

	resp, err := c.client.QueryDocuments(ctx, c.options.DatabaseName, c.options.CollectionName, *qry, &entities, qops)
	if err != nil {
		return nil, err
	}

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
	logger := ucplog.FromContextOrDiscard(ctx)
	logger.Info("cosmosdb: Get - id: %s", "resourceID", id)

	parsedID, err := resources.Parse(id)
	if err != nil {
		return nil, err
	}

	partitionKey, err := GetPartitionKey(parsedID)
	fmt.Printf("partitionKey: %s\n", partitionKey)
	if err != nil {
		return nil, err
	}

	ops := cosmosapi.GetDocumentOptions{
		PartitionKeyValue: partitionKey,
	}

	docID, err := GenerateCosmosDBKey(parsedID)
	logger.Info("cosmosdb: Get - docID: ", "docID", docID)
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
	parsedID, err := resources.Parse(id)
	if err != nil {
		return err
	}

	partitionKey, err := GetPartitionKey(parsedID)
	if err != nil {
		return err
	}

	ops := cosmosapi.DeleteDocumentOptions{
		PartitionKeyValue: partitionKey,
	}

	docID, err := GenerateCosmosDBKey(parsedID)
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
func (c *CosmosDBStorageClient) Save(ctx context.Context, obj *store.Object, opts ...store.SaveOptions) error {
	if ctx == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'ctx' is required"}
	}
	if obj == nil {
		return &store.ErrInvalid{Message: "invalid argument. 'obj' is required"}
	}

	cfg := store.NewSaveConfig(opts...)

	parsed, err := resources.Parse(obj.ID)
	if err != nil {
		return err
	}

	docID, err := GenerateCosmosDBKey(parsed)
	if err != nil {
		return err
	}

	partitionKey, err := GetPartitionKey(parsed)
	if err != nil {
		return err
	}

	entity := &ResourceEntity{
		ID:           docID,
		ResourceID:   strings.ToLower(parsed.String()),
		RootScope:    strings.ToLower(parsed.RootScope()),
		PartitionKey: partitionKey,
		Entity:       obj.Data,
	}

	ifMatch := cfg.ETag
	if ifMatch == "" && obj.ETag != "" {
		ifMatch = obj.ETag
	}

	var resp *cosmosapi.Resource
	if ifMatch == "" {
		op := cosmosapi.CreateDocumentOptions{
			PartitionKeyValue: partitionKey,
			IsUpsert:          true,
		}
		resp, _, err = c.client.CreateDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity, op)
	} else {
		op := cosmosapi.ReplaceDocumentOptions{
			PartitionKeyValue: partitionKey,
			IfMatch:           ifMatch,
		}
		resp, _, err = c.client.ReplaceDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity.ID, entity, op)

		// TODO: use the response code when switching to official SDK.
		if err != nil && strings.HasPrefix(err.Error(), errEtagPreconditionMsgPrefix) {
			return &store.ErrConcurrency{}
		}
	}

	if err != nil {
		return err
	}

	obj.ETag = resp.Etag

	return nil
}

// GetPartitionKey function returns the partition key for a resource
// Examples:
// /planes/radius/local/... - Partition Key: radius/local
// subscriptions/{guid}/... - Partition Key: {guid}
func GetPartitionKey(id resources.ID) (string, error) {
	partitionKey := NormalizeSubscriptionID(id.FindScope(resources.SubscriptionsSegment))

	if id.IsUCPQualfied() {
		partitionKey = NormalizeLetterOrDigitToUpper(id.PlaneNamespace())
	}

	return partitionKey, nil
}
