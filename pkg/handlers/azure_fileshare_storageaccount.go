// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-04-01/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/resourcemodel"
)

const (
	FileShareStorageAccountNameKey = "filesharestorageaccount"
	FileShareStorageAccountIDKey   = "filesharestorageaccountid"
)

func NewAzureFileShareStorageAccountHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureFileShareStorageAccountHandler{arm: arm}
}

type azureFileShareStorageAccountHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureFileShareStorageAccountHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, FileShareStorageAccountIDKey, FileShareStorageAccountNameKey)
	if err != nil {
		return nil, err
	}

	if properties[FileShareStorageAccountIDKey] == "" {
		// TODO Managed resource
	} else {
		_, err = getStorageAccountByID(ctx, handler.arm, properties[FileShareStorageAccountIDKey])
		if err != nil {
			return nil, err
		}
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(properties[FileShareStorageAccountIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))
	return properties, nil
}

func (handler *azureFileShareStorageAccountHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	return deleteStorageAccount(ctx, handler.arm, properties[StorageAccountNameKey])
}

func NewAzureFileShareStorageAccountHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureFileShareStorageAccountHealthHandler{arm: arm}
}

type azureFileShareStorageAccountHealthHandler struct {
	arm armauth.ArmConfig
}

func (handler *azureFileShareStorageAccountHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
