// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
)

const (
	FileShareStorageAccountNameKey       = "filesharestorageaccount"
	FileShareStorageAccountIDKey         = "filesharestorageaccountid"
	AzureFileShareStorageAccountBaseName = "storageaccountbase"
)

func NewAzureFileShareStorageAccountHandler(arm *armauth.ArmConfig) ResourceHandler {
	return &azureFileShareStorageAccountHandler{arm: arm}
}

type azureFileShareStorageAccountHandler struct {
	arm *armauth.ArmConfig
}

func (handler *azureFileShareStorageAccountHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties, ok := options.Resource.Resource.(map[string]string)
	if !ok {
		return nil, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareStorageAccountNameKey)
	if err != nil {
		return nil, err
	}

	_, err = getStorageAccountByID(ctx, *handler.arm, properties[FileShareStorageAccountIDKey])
	if err != nil {
		return nil, err
	}

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, properties[FileShareStorageAccountIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))

	return nil, nil
}

func (handler *azureFileShareStorageAccountHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}

func getStorageAccountByID(ctx context.Context, arm armauth.ArmConfig, accountID string) (*storage.Account, error) {
	parsed, err := resources.ParseResource(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	sac := clients.NewAccountsClient(parsed.FindScope(resources.SubscriptionsSegment), arm.Auth)

	account, err := sac.GetProperties(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.TypeSegments()[0].Name, storage.AccountExpand(""))
	if err != nil {
		return nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	return &account, nil
}
