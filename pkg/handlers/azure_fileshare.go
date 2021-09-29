// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-04-01/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
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

	if properties[FileShareIDKey] == "" {
		// TODO
		// Create managed resource
	} else {
		// This is mostly called for the side-effect of verifying that the database exists.
		_, err := handler.GetFileShareByID(ctx, properties[FileShareIDKey])
		if err != nil {
			return nil, err
		}

		options.Resource.Identity = resourcemodel.NewARMIdentity(properties[FileShareIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))
	}

	return properties, nil
}

func (handler *azureFileShareHandler) GetFileShareByID(ctx context.Context, fileshareID string) (*storage.FileShare, error) {
	parsed, err := azresources.Parse(fileshareID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file share resource id: %w", err)
	}

	fc := clients.NewFileSharesClient(parsed.SubscriptionID, handler.arm.Auth)
	fs, err := fc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[2].Name, "", "")
	if err != nil {
		return nil, fmt.Errorf("failed to get FileShare: %w", err)
	}
	return &fs, nil
}

func (handler *azureFileShareHandler) DeleteFileShare(ctx context.Context, fileshareID string) error {
	parsed, err := azresources.Parse(fileshareID)
	if err != nil {
		return fmt.Errorf("failed to parse file share resource id: %w", err)
	}

	fc := clients.NewFileSharesClient(parsed.SubscriptionID, handler.arm.Auth)
	_, err = fc.Delete(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[2].Name, "", "")
	if err != nil {
		return fmt.Errorf("failed to DELETE keyvault: %w", err)
	}

	return nil
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	fileshareID := properties[FileShareIDKey]

	// Delete Azure File Share
	err := handler.DeleteFileShare(ctx, fileshareID)
	if err != nil {
		return err
	}

	return nil
}

func NewAzureFileShareHealthHandler(arm armauth.ArmConfig) HealthHandler {
	return &azureCosmosDBSQLDBHealthHandler{
		azureCosmosDBBaseHandler: azureCosmosDBBaseHandler{
			arm: arm,
		},
	}
}

type azureFileShareHealthHandler struct {
	azureCosmosDBBaseHandler
}

func (handler *azureFileShareHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
