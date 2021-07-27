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

	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)
	rc.Create(ctx, handler.arm.ResourceGroup, redisName, redis.CreateParameters{})
	return properties, nil
}

func (handler *azureRedisHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	rc := azclients.NewRedisClient(handler.arm.SubscriptionID, handler.arm.Auth)
	_, err := rc.Delete(ctx, handler.arm.ResourceGroup, properties[AzureRedisNameKey])
	if err != nil {
		return err
	}

	return nil
}
