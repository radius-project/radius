// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/project-radius/radius/pkg/cli/clients"
	"github.com/project-radius/radius/pkg/connectorrp/api/v20220315privatepreview"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

//TODO: Change subId and ResourceId to scope
type ARMUCPManagementClient struct {
	Connection      *arm.Connection
	ResourceGroup   string
	SubscriptionID  string
	EnvironmentName string
}

var _ clients.FirstPartyServiceManagementClient = (*ARMUCPManagementClient)(nil)

// ListAllResourcesByApplication lists the resources of a particular application
func (um *ARMUCPManagementClient) ListAllResourcesByApplication(ctx context.Context, applicationName string) ([]v20220315privatepreview.Resource, error) {
	mongoResourceList, err := getMongoResources(um.Connection, um.SubscriptionID, um.ResourceGroup, ctx)
	if err != nil {
		return nil, err
	}
	resourceMap := make(map[string]v20220315privatepreview.Resource)
	for _, mongoResource := range mongoResourceList {
		resourceMap[*mongoResource.Properties.Application] = mongoResource.Resource
	}

	rabbitResourceList, err := getRabbitMqResources(um.Connection, um.SubscriptionID, um.ResourceGroup, ctx)
	if err != nil {
		return nil, err
	}
	for _, rabbitResource := range rabbitResourceList {
		resourceMap[*rabbitResource.Properties.Application] = rabbitResource.Resource
	}

	redisResourceList, err := getRedisResources(um.Connection, um.SubscriptionID, um.ResourceGroup, ctx)
	if err != nil {
		return nil, err
	}
	for _, redisResource := range redisResourceList {
		resourceMap[*redisResource.Properties.Application] = redisResource.Resource
	}

	sqlResourceList, err := getSQLResources(um.Connection, um.SubscriptionID, um.ResourceGroup, ctx)
	if err != nil {
		return nil, err
	}
	for _, sqlResource := range sqlResourceList {
		resourceMap[*sqlResource.Properties.Application] = sqlResource.Resource
	}
	
	return filterByName(resourceMap, applicationName)
}

// get all mongo resources
func getMongoResources(con *arm.Connection, subscriptionId string, resourceGroupName string, ctx context.Context) ([]v20220315privatepreview.MongoDatabaseResource, error) {
	
	mongoclient := v20220315privatepreview.NewMongoDatabasesClient(con, "00000000-0000-0000-0000-000000000000")
	mongoPager := mongoclient.List("radius-test-rg", nil)
	mongoResourceList := []v20220315privatepreview.MongoDatabaseResource{}
	for mongoPager.NextPage(ctx) {
		currResourceList := mongoPager.PageResponse().MongoDatabaseList.Value
		for _, resource := range currResourceList {
			mongoResourceList = append(mongoResourceList, *resource)
		}
	}
	return mongoResourceList, nil
}

// get all rabbit mq resources
func getRabbitMqResources(con *arm.Connection, subscriptionId string, resourceGroupName string, ctx context.Context) ([]v20220315privatepreview.RabbitMQMessageQueueResource, error) {
	
	rabbitClient := v20220315privatepreview.NewRabbitMQMessageQueuesClient(con, "00000000-0000-0000-0000-000000000000")
	rabbitPager := rabbitClient.List("radius-test-rg", nil)
	rabbitResourceList := []v20220315privatepreview.RabbitMQMessageQueueResource{}
	for rabbitPager.NextPage(ctx) {
		currResourceList := rabbitPager.PageResponse().RabbitMQMessageQueueList.Value
		for _, resource := range currResourceList {
			rabbitResourceList = append(rabbitResourceList, *resource)
		}
	}
	return rabbitResourceList, nil
}

// get all rabbit resources
func getRedisResources(con *arm.Connection, subscriptionId string, resourceGroupName string, ctx context.Context) ([]v20220315privatepreview.RedisCacheResource, error) {
	
	redisClient := v20220315privatepreview.NewRedisCachesClient(con, "00000000-0000-0000-0000-000000000000")
	redisPager := redisClient.List("radius-test-rg", nil)
	redisResourceList := []v20220315privatepreview.RedisCacheResource{}
	for redisPager.NextPage(ctx) {
		currResourceList := redisPager.PageResponse().RedisCacheList.Value
		for _, resource := range currResourceList {
			redisResourceList = append(redisResourceList, *resource)
		}
	}
	return redisResourceList, nil
}

// get all sql resources
func getSQLResources(con *arm.Connection, subscriptionId string, resourceGroupName string, ctx context.Context) ([]v20220315privatepreview.SQLDatabaseResource, error) {
	
	sqlClient := v20220315privatepreview.NewSQLDatabasesClient(con, "00000000-0000-0000-0000-000000000000")
	sqlPager := sqlClient.List("radius-test-rg", nil)
	sqlResourceList := []v20220315privatepreview.SQLDatabaseResource{}
	for sqlPager.NextPage(ctx) {
		currResourceList := sqlPager.PageResponse().SQLDatabaseList.Value
		for _, resource := range currResourceList {
			sqlResourceList = append(sqlResourceList, *resource)
		}
	}
	return sqlResourceList, nil
}

func filterByName(resourceList map[string]v20220315privatepreview.Resource, applicationName string) ([]v20220315privatepreview.Resource, error){
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

func getResourceAppName(applicationId string) (string, error) {
	parsedId, err := resources.Parse(applicationId)
	if err != nil {
		return "", err
	}
	return parsedId.Name(), nil
}
