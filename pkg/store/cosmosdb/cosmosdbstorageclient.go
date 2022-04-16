// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package cosmosdb

import (
	"context"
	"fmt"
	"net/http"

	"github.com/pkg/errors"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/store"
	"github.com/vippsas/go-cosmosdb/cosmosapi"
)

const (
	// PartitionKeyName is the property used for partitioning.
	PartitionKeyName = "/partitionKey"
)

var _ store.StorageClient = (*CosmosDBStorageClient)(nil)

// ResourceEntity represents the default envelope model to store resource metadata.
type ResourceEntity struct {
	// ID represents the primary key.
	ID string `json:"id"`
	// ResourceID represents fully qualified resource id
	ResourceID string `json:"resourceId"`
	// ETag represents an etag required for optimistic concurrency control.
	ETag string `json:"_etag"`
	// Self represents the unique addressable URI for the resource.
	Self string `json:"_self"`
	// Timestamp represents the last updated timestamp of the resource.
	UpdatedTime int `json:"_ts"`

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
	// TODO: Create the custom client transport to support service principal and managed identity.
	cfg := cosmosapi.Config{MaxRetries: 5}

	if options.KeyAuth != nil {
		cfg.MasterKey = options.KeyAuth.MasterKey
	} else if options.AzureADAuth != nil {
		cfg.MasterKey = "aad" // We set the fake key for Azure AD authentication. Since go-cosmosdb doesn't support Azure AD, CustomTransport takes care of AzureAD JWT acquisition.
	} else {
		return nil, errors.New("unknown authentication options")
	}

	httpClient, err := getHTTPClient(options)
	if err != nil {
		return nil, errors.New("fails to get HTTPClient")
	}

	client := cosmosapi.New(options.Url, cfg, httpClient, nil)

	return &CosmosDBStorageClient{
		client:  client,
		options: options,
	}, nil
}

func getHTTPClient(options *ConnectionOptions) (*http.Client, error) {
	clientTransport := http.DefaultTransport

	if options.AzureADAuth != nil {
		var err error
		clientTransport, err = NewClientTransport(options.AzureADAuth)
		if err != nil {
			return nil, err
		}
	}

	return &http.Client{
		Transport: clientTransport,
	}, nil
}

// Init initializes the database and collection.
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

func constructCosmosDBQuery(query store.Query) (*ResourceScope, *cosmosapi.Query, error) {
	// Validate query request.
	resourceScope, err := NewResourceScope(query.RootScope)
	if err != nil {
		return nil, nil, err
	}

	if resourceScope.SubscriptionID == "" {
		return nil, nil, &store.ErrInvalid{Message: "invalid RootScope"}
	}

	if query.RoutingScopePrefix != "" {
		return nil, nil, &store.ErrInvalid{Message: "RoutingScopePrefix is not supported"}
	}

	// Construct SQL query
	queryString := "SELECT * FROM c WHERE c.entity.subscriptionID = @subId"
	queryParams := []cosmosapi.QueryParam{
		{
			Name:  "@subId",
			Value: NormalizeSubscriptionID(resourceScope.SubscriptionID),
		},
	}

	if resourceScope.ResourceGroup != "" {
		queryString = queryString + " and c.entity.resourceGroup = @rgName"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rgName",
			Value: NormalizeSubscriptionID(resourceScope.ResourceGroup),
		})
	}

	if query.ResourceType != "" {
		queryString = queryString + " and STRINGEQUALS(c.entity.type, @rtype, true)"
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@rtype",
			Value: query.ResourceType,
		})
	}

	for i, filter := range query.Filters {
		filterParam := fmt.Sprintf("filter%d", i)
		queryString = queryString + fmt.Sprintf(" and STRINGEQUALS(c.entity.%s, @%s, true)", filter.Field, filterParam)
		queryParams = append(queryParams, cosmosapi.QueryParam{
			Name:  "@" + filterParam,
			Value: filter.Value,
		})
	}

	return resourceScope, &cosmosapi.Query{Query: queryString, Params: queryParams}, nil
}

// Query queries the data resource
func (c *CosmosDBStorageClient) Query(ctx context.Context, query store.Query, options ...store.QueryOptions) ([]store.Object, error) {
	// Prepare document query.
	resourceScope, qry, err := constructCosmosDBQuery(query)
	if err != nil {
		return nil, err
	}

	entities := []ResourceEntity{}

	qops := cosmosapi.QueryDocumentsOptions{
		PartitionKeyValue:    NormalizeSubscriptionID(resourceScope.SubscriptionID),
		IsQuery:              true,
		ContentType:          cosmosapi.QUERY_CONTENT_TYPE,
		MaxItemCount:         20,    // TODO: move this count to query options.
		EnableCrossPartition: false, // TODO: enable when we need to query across subscriptions.
		ConsistencyLevel:     cosmosapi.ConsistencyLevelEventual,
	}

	if query.ContinuationToken != "" {
		qops.Continuation = query.ContinuationToken
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
				ID:                entity.ResourceID,
				ETag:              entity.ETag,
				ContinuationToken: resp.Continuation,
			},
			Data: entity.Entity,
		})
	}

	return output, nil
}

// Get gets the resource data using id.
func (c *CosmosDBStorageClient) Get(ctx context.Context, id string, options ...store.GetOptions) (*store.Object, error) {
	azID, err := azresources.Parse(id)
	if err != nil {
		return nil, err
	}

	ops := cosmosapi.GetDocumentOptions{
		PartitionKeyValue: NormalizeSubscriptionID(azID.SubscriptionID),
	}

	docID := GenerateResourceID(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	entity := &ResourceEntity{}
	_, err = c.client.GetDocument(ctx, c.options.DatabaseName, c.options.CollectionName, docID, ops, entity)

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
	docID := GenerateResourceID(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name())
	_, err = c.client.DeleteDocument(ctx, c.options.DatabaseName, c.options.CollectionName, docID, ops)

	return errors.WithStack(err)
}

// Save upserts the resource.
func (c *CosmosDBStorageClient) Save(ctx context.Context, obj *store.Object, options ...store.SaveOptions) (*store.Object, error) {
	azID, err := azresources.Parse(obj.ID)
	if err != nil {
		return nil, err
	}

	entity := &ResourceEntity{
		ID:           GenerateResourceID(azID.SubscriptionID, azID.ResourceGroup, azID.Type(), azID.Name()),
		ResourceID:   azID.ID,
		PartitionKey: NormalizeSubscriptionID(azID.SubscriptionID),
		Entity:       obj.Data,
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
			IfMatch:           obj.ETag,
		}
		resp, _, err = c.client.ReplaceDocument(ctx, c.options.DatabaseName, c.options.CollectionName, entity.ID, entity, op)
	}

	if err != nil {
		return nil, &store.ErrInvalid{Message: err.Error()}
	}

	obj.ETag = resp.Etag

	return obj, nil
}
