// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/redis/mgmt/redis"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	RedisBaseName      = "azureredis"
	RedisNameKey       = "redisname"
	RedisResourceIdKey = "redisid"
	RedisPortKey       = "redisport"
	RedisHostKey       = "redishost"
	RedisUsernameKey   = "redisusername"
	// On Azure, RedisUsername is empty.
	RedisUsername            = ""
	RedisConnectionStringKey = "redisconnectionstring"
	RedisPasswordKey         = "redispassword"
)

func NewAzureRedisHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureRedisHandler{
		arm: arm,
	}
}

type azureRedisHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureRedisHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("missing required properties for resource")
	}
	parsedID, err := resources.Parse(properties[RedisResourceIdKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to parse CosmosDB Mongo Database resource id: %w", err)
	}
	redisClient := clients.NewRedisClient(parsedID.FindScope(resources.SubscriptionsSegment), handler.arm.Auth)
	cache, err := redisClient.Get(ctx, parsedID.FindScope(resources.ResourceGroupsSegment), properties[RedisNameKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to get redis cache: %w", err)
	}
	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *cache.ID, clients.GetAPIVersionFromUserAgent(redis.UserAgent()))

	// Properties that are referenced from the renderer
	properties[RedisNameKey] = *cache.Name
	properties[RedisHostKey] = *cache.HostName
	properties[RedisPortKey] = fmt.Sprintf("%d", *cache.Properties.SslPort)
	properties[RedisUsernameKey] = RedisUsername

	return outputResourceIdentity, properties, nil
}

func (handler *azureRedisHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	return nil
}
