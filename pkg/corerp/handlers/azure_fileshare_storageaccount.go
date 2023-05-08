/*
------------------------------------------------------------
Copyright 2023 The Radius Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
------------------------------------------------------------
*/

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
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

	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, properties[FileShareStorageAccountIDKey], clientv2.StateStoreClientAPIVersion)

	return nil, nil
}

func (handler *azureFileShareStorageAccountHandler) Delete(ctx context.Context, options *DeleteOptions) error {
	return nil
}

func getStorageAccountByID(ctx context.Context, arm armauth.ArmConfig, accountID string) (*armstorage.Account, error) {
	parsed, err := resources.ParseResource(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	client, err := clientv2.NewAccountsClient(parsed.FindScope(resources.SubscriptionsSegment), &arm.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Storage Account client: %w", err)
	}

	resp, err := client.GetProperties(ctx, parsed.FindScope(resources.ResourceGroupsSegment),
		parsed.TypeSegments()[0].Name, &armstorage.AccountsClientGetPropertiesOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	return &resp.Account, nil
}
