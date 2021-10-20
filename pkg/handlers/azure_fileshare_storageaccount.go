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
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
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

	var account *storage.Account
	if properties[FileShareStorageAccountIDKey] == "" {
		generated, err := generateStorageAccountName(ctx, handler.arm, properties[ResourceName])
		if err != nil {
			return nil, err
		}

		name := *generated

		account, err = handler.CreateStorageAccount(ctx, name, *options)
		if err != nil {
			return nil, err
		}

		// store storage account so we can delete later
		properties[FileShareStorageAccountNameKey] = *account.Name
		properties[FileShareStorageAccountIDKey] = *account.ID
	} else {
		_, err = getStorageAccountByID(ctx, handler.arm, properties[FileShareStorageAccountIDKey])
		return nil, err
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

	return deleteStorageAccount(ctx, handler.arm, properties[FileShareStorageAccountNameKey])
}

func (handler *azureFileShareStorageAccountHandler) CreateStorageAccount(ctx context.Context, accountName string, options PutOptions) (*storage.Account, error) {
	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	sc := clients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sc.Create(ctx, handler.arm.ResourceGroup, accountName, storage.AccountCreateParameters{
		Location: location,
		Tags:     keys.MakeTagsForRadiusResource(options.ApplicationName, options.ResourceName),
		Kind:     storage.KindStorageV2,
		Sku: &storage.Sku{
			Name: storage.SkuNameStandardLRS,
		},
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create/update storage account: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update storage account: %w", err)
	}

	account, err := future.Result(sc)
	if err != nil {
		return nil, fmt.Errorf("failed to create/update storage account: %w", err)
	}

	return &account, nil
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
