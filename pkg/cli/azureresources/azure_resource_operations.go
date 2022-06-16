// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azureresources

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

type AzureResourceType int

const (
	MongoResource AzureResourceType = iota
	RabbitMQResource
	RedisResource
	SQLResource
)

type AzureResourceOperationsModel struct {
	ResourceType       AzureResourceType
	ResourceOperations AzureResourceOperations
}

// AzureResourceOperations is used to interface with resource operations like listing resources by app, show details of a resource.
type AzureResourceOperations interface {
	GetResourcesByApplication(con *arm.Connection, ctx context.Context, rootScope string, applicationName string) ([]v20220315privatepreview.Resource, error)
}

var _ AzureResourceOperations = (*MongoResourceOperations)(nil)

type MongoResourceOperations struct {
}

func (mo *MongoResourceOperations) GetResourcesByApplication(con *arm.Connection, ctx context.Context, rootScope string, applicationName string) ([]v20220315privatepreview.Resource, error) {
	mongoClient := v20220315privatepreview.NewMongoDatabasesClient(con, "/subscriptionId/00000000-0000-0000-0000-000000000000/resourceGroup/radius-test-rg")
	mongoPager := mongoClient.ListByRootScope(nil)
	resourceMap := make(map[string]v20220315privatepreview.Resource)
	for mongoPager.NextPage(ctx) {
		mongoResourceList := mongoPager.PageResponse().MongoDatabaseList.Value
		for _, mongoResource := range mongoResourceList {
			resourceMap[*mongoResource.Properties.Application] = mongoResource.Resource
		}
	}
	return filterByApplicationName(resourceMap, applicationName)
}

var _ AzureResourceOperations = (*RabbitResourceOperations)(nil)

type RabbitResourceOperations struct {
}

func (ro *RabbitResourceOperations) GetResourcesByApplication(con *arm.Connection, ctx context.Context, rootScope string, applicationName string) ([]v20220315privatepreview.Resource, error) {
	rabbitClient := v20220315privatepreview.NewRabbitMQMessageQueuesClient(con, rootScope)
	rabbitPager := rabbitClient.ListByRootScope(nil)
	resourceMap := make(map[string]v20220315privatepreview.Resource)
	for rabbitPager.NextPage(ctx) {
		mongoResourceList := rabbitPager.PageResponse().RabbitMQMessageQueueList.Value
		for _, mongoResource := range mongoResourceList {
			resourceMap[*mongoResource.Properties.Application] = mongoResource.Resource
		}
	}
	return filterByApplicationName(resourceMap, applicationName)
}

var _ AzureResourceOperations = (*RedisResourceOperations)(nil)

type RedisResourceOperations struct {
}

func (ro *RedisResourceOperations) GetResourcesByApplication(con *arm.Connection, ctx context.Context, rootScope string, applicationName string) ([]v20220315privatepreview.Resource, error) {
	redisClient := v20220315privatepreview.NewRedisCachesClient(con, rootScope)
	redisPager := redisClient.ListByRootScope(nil)
	resourceMap := make(map[string]v20220315privatepreview.Resource)
	for redisPager.NextPage(ctx) {
		mongoResourceList := redisPager.PageResponse().RedisCacheList.Value
		for _, mongoResource := range mongoResourceList {
			resourceMap[*mongoResource.Properties.Application] = mongoResource.Resource
		}
	}
	return filterByApplicationName(resourceMap, applicationName)
}

var _ AzureResourceOperations = (*SQLResourceOperations)(nil)

type SQLResourceOperations struct {
}

func (ro *SQLResourceOperations) GetResourcesByApplication(con *arm.Connection, ctx context.Context, rootScope string, applicationName string) ([]v20220315privatepreview.Resource, error) {
	sqlClient := v20220315privatepreview.NewSQLDatabasesClient(con, rootScope)
	sqlPager := sqlClient.ListByRootScope(nil)
	resourceMap := make(map[string]v20220315privatepreview.Resource)
	for sqlPager.NextPage(ctx) {
		mongoResourceList := sqlPager.PageResponse().SQLDatabaseList.Value
		for _, mongoResource := range mongoResourceList {
			resourceMap[*mongoResource.Properties.Application] = mongoResource.Resource
		}
	}
	return filterByApplicationName(resourceMap, applicationName)
}

func filterByApplicationName(resourceList map[string]v20220315privatepreview.Resource, applicationName string) ([]v20220315privatepreview.Resource, error) {
	filteredResourceList := []v20220315privatepreview.Resource{}
	for appId, resource := range resourceList {
		IdParsed, err := resources.Parse(appId)
		if err != nil {
			return nil, err
		}
		if IdParsed.Name() == applicationName {
			filteredResourceList = append(filteredResourceList, resource)
		}
	}
	return filteredResourceList, nil
}
