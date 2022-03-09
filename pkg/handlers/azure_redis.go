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
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/resourcemodel"
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

func (handler *azureRedisHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	var redisResource *redis.ResourceType

	var err error
	redisResource, err = handler.GetRedisByID(ctx, properties[RedisResourceIdKey])
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(*redisResource.ID, clients.GetAPIVersionFromUserAgent(redis.UserAgent()))

	// Properties that are referenced from the renderer
	properties[RedisNameKey] = *redisResource.Name
	properties[RedisHostKey] = *redisResource.HostName
	properties[RedisPortKey] = fmt.Sprintf("%d", *redisResource.Properties.SslPort)
	properties[RedisUsernameKey] = RedisUsername

	return properties, nil
}

func (handler *azureRedisHandler) Delete(ctx context.Context, options DeleteOptions) error {
	return nil
}

func (handler *azureRedisHandler) GetRedisByID(ctx context.Context, id string) (*redis.ResourceType, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis resource id: %w", err)
	}

	rc := clients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	redis, err := rc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get redis: %w", err)
	}
	return &redis, nil
}

func NewAzureRedisHealthHandler(arm *armauth.ArmConfig) HealthHandler {
	return &azureRedisHealthHandler{
		arm: arm,
	}
}

type azureRedisHealthHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureRedisHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
