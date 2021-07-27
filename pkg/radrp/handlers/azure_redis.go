// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/redis/mgmt/2018-03-01/redis"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/radrp/armauth"
)

const (
	AzureRedisNameKey = "azureredis"
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

	redisName, ok := properties[AzureRedisNameKey]
	if !ok {
		return nil, fmt.Errorf("missing required property '%s'", AzureRedisNameKey)
	}

	_, err := handler.CreateRedis(ctx, redisName)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *azureRedisHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	err := handler.DeleteRedis(ctx, properties[AzureRedisNameKey])
	if err != nil {
		return err
	}

	return nil
}

func (handler *azureRedisHandler) CreateRedis(ctx context.Context, redisName string) (*redis.ResourceType, error) {
	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)
	createFuture, err := rc.Create(ctx, handler.arm.ResourceGroup, redisName, redis.CreateParameters{})
	if err != nil {
		return nil, fmt.Errorf("failed to create redis: %w", err)
	}

	err = createFuture.WaitForCompletionRef(ctx, rc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb database: %w", err)
	}

	resourceType, err := createFuture.Result(rc)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update cosmosdb database: %w", err)
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
