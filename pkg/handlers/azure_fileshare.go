// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"

	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/healthcontract"
)

const (
	FileShareNameKey               = "fileshare"
	FileShareIDKey                 = "fileshareid"
	FileShareStorageAccountIDKey   = "storageaccountkey"
	FileShareStorageAccountNameKey = "namekey"
)

func NewAzureFileShareHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureFileShareHandler{arm: arm}
}

type azureFileShareHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureFileShareHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, FileShareStorageAccountIDKey, FileShareIDKey)
	if err != nil {
		return nil, err
	}

	if properties[FileShareIDKey] == "" {
		// TODO
		// Create managed resource
		err = errors.New("Managed Azure File share is not yet supported")
	} else {
		armhandler := NewARMHandler(handler.arm)
		properties, err = armhandler.Put(ctx, options)
		if err != nil {
			return nil, err
		}
	}
	return properties, err
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	armHandler := NewARMHandler(handler.arm)
	return armHandler.Delete(ctx, options)
}

func NewAzureFileShareHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureFileShareHealthHandler{arm: arm}
}

type azureFileShareHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureFileShareHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
