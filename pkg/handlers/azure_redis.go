// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/redis/mgmt/redis"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
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
	RedisUsername = ""
)

func NewAzureRedisHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureRedisHandler{
		arm: arm,
	}
}

type azureRedisHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRedisHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	var redisResource *redis.ResourceType
	if properties[RedisResourceIdKey] == "" {
		// If we don't have an ID already, then we need to create a new Redis.
		redisName, ok := properties[RedisNameKey]
		var err error
		if !ok {
			redisName, err = generateUniqueAzureResourceName(ctx,
				properties[RedisBaseName],
				func(name string) error {
					rc := clients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)
					redisType := "Microsoft.Cache/redis"
					checkNameParams := redis.CheckNameAvailabilityParameters{
						Name: &name,
						Type: &redisType,
					}

					checkNameResult, err := rc.CheckNameAvailability(ctx, checkNameParams)
					if err != nil {
						return err
					}

					if checkNameResult.StatusCode != 200 {
						return fmt.Errorf("name not available with status code: %v", checkNameResult.StatusCode)
					}
					return nil
				})

			if err != nil {
				return nil, err
			}
		}

		redisResource, err = handler.CreateRedis(ctx, redisName)
		if err != nil {
			return nil, err
		}
		properties[RedisResourceIdKey] = *redisResource.ID
	} else {
		var err error
		redisResource, err = handler.GetRedisByID(ctx, properties[RedisResourceIdKey])
		if err != nil {
			return nil, err
		}
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
	properties := options.ExistingOutputResource.PersistedProperties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	err := handler.DeleteRedis(ctx, properties[RedisBaseName])
	if err != nil {
		return err
	}

	return nil
}

func (handler *azureRedisHandler) CreateRedis(ctx context.Context, redisName string) (*redis.ResourceType, error) {
	rc := clients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// Basic redis SKU
	redisSku := &redis.Sku{
		Name:     redis.SkuName("Basic"),
		Family:   redis.SkuFamily("C"),
		Capacity: to.Int32Ptr(1),
	}

	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	createParams := redis.CreateParameters{
		Location: location,
		CreateProperties: &redis.CreateProperties{
			Sku:          redisSku,
			RedisVersion: to.StringPtr("6"),
		},
	}

	createFuture, err := rc.Create(ctx, handler.arm.ResourceGroup, redisName, createParams)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis: %w", err)
	}

	err = createFuture.WaitForCompletionRef(ctx, rc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis: %w", err)
	}

	resourceType, err := createFuture.Result(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to create redis: %w", err)
	}

	return &resourceType, nil
}

func (handler *azureRedisHandler) DeleteRedis(ctx context.Context, redisName string) error {
	rc := clients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := rc.Delete(ctx, handler.arm.ResourceGroup, redisName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "redis", err)
	}

	err = future.WaitForCompletionRef(ctx, rc.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "redis", err)
	}

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

func NewAzureRedisHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureRedisHealthHandler{
		arm: arm,
	}
}

type azureRedisHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRedisHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
