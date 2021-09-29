// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-04-01/storage"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcemodel"
	"github.com/gofrs/uuid"
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
		generated, err := handler.GenerateStorageAccountName(ctx, properties[ComponentNameKey])
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
		account, err = handler.GetStorageAccountByID(ctx, properties[FileShareStorageAccountIDKey])
		if err != nil {
			return nil, err
		}
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(properties[FileShareStorageAccountIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))
	return properties, nil
}

func (handler *azureFileShareStorageAccountHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties
	accountName := properties[StorageAccountNameKey]

	sc := clients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	_, err := sc.Delete(ctx, handler.arm.ResourceGroup, accountName)
	if err != nil {
		return fmt.Errorf("failed to delete storage account: %w", err)
	}

	return nil
}

func (handler *azureFileShareStorageAccountHandler) GenerateStorageAccountName(ctx context.Context, baseName string) (*string, error) {
	logger := radlogger.GetLogger(ctx)
	sc := clients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// names are kinda finicky here - they have to be unique across azure.
	name := ""

	for i := 0; i < 10; i++ {
		// 3-24 characters - all alphanumeric
		uid, err := uuid.NewV4()
		if err != nil {
			return nil, fmt.Errorf("failed to generate storage account name: %w", err)
		}
		name = baseName + strings.ReplaceAll(uid.String(), "-", "")
		name = name[0:24]

		result, err := sc.CheckNameAvailability(ctx, storage.AccountCheckNameAvailabilityParameters{
			Name: to.StringPtr(name),
			Type: to.StringPtr(azresources.StorageStorageAccounts),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to query storage account name: %w", err)
		}

		if result.NameAvailable != nil && *result.NameAvailable {
			return &name, nil
		}

		logger.Info(fmt.Sprintf("storage account name generation failed: %v %v", result.Reason, result.Message))
	}

	return nil, fmt.Errorf("failed to find a storage account name")
}

func (handler *azureFileShareStorageAccountHandler) GetStorageAccountByID(ctx context.Context, accountID string) (*storage.Account, error) {
	parsed, err := azresources.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	sac := clients.NewAccountsClient(parsed.SubscriptionID, handler.arm.Auth)

	account, err := sac.GetProperties(ctx, parsed.ResourceGroup, parsed.Types[0].Name, storage.AccountExpand(""))
	if err != nil {
		return nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	return &account, nil
}

func (handler *azureFileShareStorageAccountHandler) CreateStorageAccount(ctx context.Context, accountName string, options PutOptions) (*storage.Account, error) {
	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	sc := clients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sc.Create(ctx, handler.arm.ResourceGroup, accountName, storage.AccountCreateParameters{
		Location: location,
		Tags:     keys.MakeTagsForRadiusComponent(options.Application, options.Component),
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
