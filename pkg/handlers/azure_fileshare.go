// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-10-01/resources"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-04-01/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
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
		_, err := handler.GetFileShareByID(ctx, options, properties[FileShareIDKey])
		if err != nil {
			return nil, err
		}

		options.Resource.Identity = resourcemodel.NewARMIdentity(properties[FileShareIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))
	}

	return properties, nil
}

func (handler *azureFileShareHandler) GetFileShareByID(ctx context.Context, options *PutOptions, fileshareID string) (*storage.FileShare, error) {
	// We only support user-managed resources. Do a GET just to validate that the resource exists.
	if options.Resource.Managed {
		return nil, fmt.Errorf("ARM handler only supports user-managed resources")
	}

	id, apiVersion, err := options.Resource.Identity.RequireARM()
	if err != nil {
		return nil, err
	}

	rc := clients.NewGenericResourceClient(handler.arm.SubscriptionID, handler.arm.Auth)
	resource, err := rc.GetByID(ctx, id, apiVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to access resource %q", id)
	}

	// Return the resource so renderers can use it for computed values.
	serialized, err := handler.serializeResource(resource)
	if err != nil {
		return nil, err
	}
	return serialized, nil
}

func (handler *azureFileShareHandler) serializeResource(resource resources.GenericResource) (*storage.FileShare, error) {
	b, err := json.Marshal(&resource)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %T", resource)
	}

	data := storage.FileShare{}
	err = json.Unmarshal(b, &data)
	if err != nil {
		return nil, errors.New("failed to umarshal resource data")
	}

	return &data, nil
}

func (handler *azureFileShareHandler) DeleteFileShare(ctx context.Context, accountName, fileshareName string) error {
	fc := clients.NewFileSharesClient(handler.arm.SubscriptionID, handler.arm.Auth)
	_, err := fc.Delete(ctx, handler.arm.ResourceGroup, accountName, fileshareName, "", "")
	if err != nil {
		return fmt.Errorf("failed to DELETE file share: %w", err)
	}

	return nil
}

func (handler *azureFileShareHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	// Delete Azure File Share
	err := handler.DeleteFileShare(ctx, properties[FileShareStorageAccountNameKey], properties[FileShareNameKey])
	if err != nil {
		return err
	}

	return nil
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
