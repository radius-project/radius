// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/resourcemodel"
)

const (
	FileShareNameKey = "fileshare"
	FileShareIDKey   = "fileshareid"
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

	var storageAccountName string
	for _, resource := range options.Dependencies {
		if resource.LocalID == outputresource.LocalIDAzureFileShareStorageAccount {
			storageAccountName = resource.Properties[FileShareStorageAccountNameKey]
		}
	}

	if properties, ok := options.DependencyProperties[outputresource.LocalIDAzureFileShareStorageAccount]; ok {
		storageAccountName = properties[FileShareStorageAccountNameKey]
	}

	if properties[FileShareIDKey] == "" {
		fsc := clients.NewFileSharesClient(handler.arm.SubscriptionID, handler.arm.Auth)
		fileshare, err := fsc.Create(ctx, handler.arm.ResourceGroup, storageAccountName, properties[FileShareNameKey], storage.FileShare{}, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create a file share with err: %w", err)
		}
		properties[FileShareIDKey] = *fileshare.ID
	} else {
		armhandler := NewARMHandler(handler.arm)
		properties, err = armhandler.Put(ctx, options)
		if err != nil {
			return nil, err
		}
	}
	options.Resource.Identity = resourcemodel.NewARMIdentity(properties[FileShareIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))

	return properties, nil
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
