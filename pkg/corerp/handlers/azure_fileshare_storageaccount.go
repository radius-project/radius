// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/storage/mgmt/2021-04-01/storage"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
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

func (handler *azureFileShareStorageAccountHandler) Put(ctx context.Context, resource *outputresource.OutputResource) error {
	identity, err := handler.GetResourceIdentity(ctx, *resource)
	if err != nil {
		return err
	}
	resource.Identity = identity
	return nil
}

func (handler *azureFileShareStorageAccountHandler) Delete(ctx context.Context, resource outputresource.OutputResource) error {
	return nil
}

func (handler *azureFileShareStorageAccountHandler) GetResourceIdentity(ctx context.Context, resource outputresource.OutputResource) (resourcemodel.ResourceIdentity, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, FileShareStorageAccountIDKey, FileShareStorageAccountNameKey)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}

	_, err = getStorageAccountByID(ctx, *handler.arm, properties[FileShareStorageAccountIDKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, err
	}
	identity := resourcemodel.NewARMIdentity(&resource.ResourceType, properties[FileShareStorageAccountIDKey], clients.GetAPIVersionFromUserAgent(storage.UserAgent()))
	return identity, nil
}

func (handler *azureFileShareStorageAccountHandler) GetResourceNativeIdentityKeyProperties(ctx context.Context, resource outputresource.OutputResource) (map[string]string, error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return properties, fmt.Errorf("invalid required properties for resource")
	}

	return properties, nil
}

func getStorageAccountByID(ctx context.Context, arm armauth.ArmConfig, accountID string) (*storage.Account, error) {
	parsed, err := resources.Parse(accountID)
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
