// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/preview/redis/mgmt/redis"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armauth"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
	"github.com/gofrs/uuid"
)

const (
	RedisBaseName      = "azureredis"
	RedisNameKey       = "redisname"
	RedisResourceIdKey = "redisid"
)

func NewAzureRedisHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureRedisHandler{
		arm: arm,
	}
}

type azureRedisHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureRedisHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	if properties[RedisResourceIdKey] == "" {
		// If we don't have an ID already, then we need to create a new Redis.
		redisName, err := handler.generateRedisDBAccountName(ctx, properties)
		properties[RedisNameKey] = redisName
		if err != nil {
			return nil, err
		}

		r, err := handler.CreateRedis(ctx, redisName)
		if err != nil {
			return nil, err
		}
		properties[RedisResourceIdKey] = *r.ID
	} else {
		_, err := handler.GetRedisByID(ctx, properties[RedisResourceIdKey])
		if err != nil {
			return nil, err
		}
	}

	return properties, nil
}

func (handler *azureRedisHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
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
	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// TODO https://github.com/Azure/radius/issues/795
	// we want to be able to create a Redis 6 cluster.

	// Basic redis SKU
	redisSku := &redis.Sku{
		Name:     redis.SkuName("Basic"),
		Family:   redis.SkuFamily("C"),
		Capacity: to.Int32Ptr(1),
	}

	location, err := getResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	createParams := redis.CreateParameters{
		Location: location,
		CreateProperties: &redis.CreateProperties{
			Sku: redisSku,
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
	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	deletefuture, err := rc.Delete(ctx, handler.arm.ResourceGroup, redisName)
	if err != nil && deletefuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete Redis: %w", err)
	}
	err = deletefuture.WaitForCompletionRef(ctx, rc.Client)
	if err != nil && !util.IsAutorest404Error(err) {
		return fmt.Errorf("failed to delete Redis: %w", err)
	}

	response, err := deletefuture.Result(rc)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to delete Redis: %w", err)
	}

	return nil
}

func (handler *azureRedisHandler) GetRedisByID(ctx context.Context, id string) (*redis.ResourceType, error) {
	parsed, err := radresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis resource id: %w", err)
	}

	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	redis, err := rc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get redis: %w", err)
	}
	return &redis, nil
}

// generateRedisDBAccountName generates account name with the specified database name as prefix appended with -<uuid>.
// This is needed since CosmosDB account names are required to be unique across Azure.
func (handler *azureRedisHandler) generateRedisDBAccountName(ctx context.Context,
	properties map[string]string) (string, error) {
	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)

	logger := radlogger.GetLogger(ctx)
	retryAttempts := 10
	name, ok := properties[CosmosDBAccountNameKey]
	if !ok {
		// properties[AzureRedisNameKey] is the component name passed through the template, this is used as a prefix for the account name
		base := properties[RedisBaseName] + "-"
		name = ""

		for i := 0; i < retryAttempts; i++ {
			// 3-24 characters - all alphanumeric and '-'
			uid, err := uuid.NewV4()
			if err != nil {
				return "", fmt.Errorf("failed to generate CosmosDB account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			redisType := "Microsoft.Cache/redis"
			checkNameParams := redis.CheckNameAvailabilityParameters{
				Name: &name,
				Type: &redisType,
			}

			checkNameResult, err := rc.CheckNameAvailability(ctx, checkNameParams)
			if err != nil {
				return "", err
			}

			if checkNameResult.StatusCode == 200 {
				return name, nil
			}

			logger.Info(fmt.Sprintf("cosmosDB account name generation failed after %d attempts", i))
		}

		return "", fmt.Errorf("cosmosDB account name generation failed to create a unique name after %d attempts", retryAttempts)
	}

	return name, nil
}
