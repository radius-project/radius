// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/azclients"
	"github.com/Azure/radius/pkg/azresources"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/radrp/armauth"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
	"github.com/gofrs/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StorageAccountNameKey = "storageaccount"
	StorageAccountIDKey   = "storageaccountid"
)

func NewDaprStateStoreAzureStorageHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreAzureStorageHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreAzureStorageHandler struct {
	kubernetesHandler
	arm armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreAzureStorageHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, StorageAccountIDKey)
	if err != nil {
		return nil, err
	}

	var account *storage.Account
	if properties[StorageAccountIDKey] == "" {
		generated, err := handler.GenerateStorageAccountName(ctx, properties[ComponentNameKey])
		if err != nil {
			return nil, err
		}

		name := *generated

		account, err = handler.CreateStorageAccount(ctx, name, options)
		if err != nil {
			return nil, err
		}

		// store storage account so we can delete later
		properties[StorageAccountNameKey] = *account.Name
		properties[StorageAccountIDKey] = *account.ID
	} else {
		account, err = handler.GetStorageAccountByID(ctx, properties[StorageAccountIDKey])
		if err != nil {
			return nil, err
		}
	}

	key, err := handler.FindStorageKey(ctx, *account.Name)
	if err != nil {
		return nil, err
	}

	err = handler.CreateDaprStateStore(ctx, *account.Name, *key.Value, properties, options)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprStateStoreAzureStorageHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	accountName := properties[StorageAccountNameKey]

	err := handler.DeleteDaprStateStore(ctx, properties)
	if err != nil {
		return err
	}

	if properties[ManagedKey] == "true" {
		err = handler.DeleteStorageAccount(ctx, accountName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (handler *daprStateStoreAzureStorageHandler) GenerateStorageAccountName(ctx context.Context, baseName string) (*string, error) {
	logger := radlogger.GetLogger(ctx)
	sc := azclients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

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

func (handler *daprStateStoreAzureStorageHandler) GetStorageAccountByID(ctx context.Context, accountID string) (*storage.Account, error) {
	parsed, err := radresources.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	sac := azclients.NewAccountsClient(parsed.SubscriptionID, handler.arm.Auth)

	account, err := sac.GetProperties(ctx, parsed.ResourceGroup, parsed.Types[0].Name, storage.AccountExpand(""))
	if err != nil {
		return nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	return &account, nil
}

func (handler *daprStateStoreAzureStorageHandler) CreateStorageAccount(ctx context.Context, accountName string, options PutOptions) (*storage.Account, error) {
	location, err := getResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	sc := azclients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sc.Create(ctx, handler.arm.ResourceGroup, accountName, storage.AccountCreateParameters{
		Location: location,
		Tags:     keys.MakeTagsForRadiusComponent(options.Application, options.Component),
		Kind:     storage.StorageV2,
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
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

func (handler *daprStateStoreAzureStorageHandler) CreateDaprStateStore(ctx context.Context, accountName string, accountKey string, properties map[string]string, options PutOptions) error {
	err := handler.PatchNamespace(ctx, properties[KubernetesNamespaceKey])
	if err != nil {
		return err
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ComponentNameKey],
				"labels":    keys.MakeDescriptiveLabels(options.Application, options.Component),
			},
			"spec": map[string]interface{}{
				"type":    "state.azure.tablestorage",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "accountName",
						"value": accountName,
					},
					map[string]interface{}{
						"name":  "accountKey",
						"value": accountKey,
					},
					map[string]interface{}{
						"name":  "tableName",
						"value": "dapr",
					},
				},
			},
		},
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: keys.FieldManager})
	if err != nil {
		return fmt.Errorf("failed to create/update Dapr Component: %w", err)
	}

	return err
}

func (handler *daprStateStoreAzureStorageHandler) FindStorageKey(ctx context.Context, accountName string) (*storage.AccountKey, error) {
	sc := azclients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	keys, err := sc.ListKeys(ctx, handler.arm.ResourceGroup, accountName, "")
	if err != nil {
		return nil, fmt.Errorf("failed to access keys of storage account: %w", err)
	}

	// Since we're doing this programmatically, let's make sure we can find a key with write access.
	if keys.Keys == nil || len(*keys.Keys) == 0 {
		return nil, fmt.Errorf("listkeys returned an empty or nil list of keys")
	}

	// Don't rely on the order the keys are in, we need Full access
	for _, k := range *keys.Keys {
		if strings.EqualFold(string(k.Permissions), string(storage.Full)) {
			key := k
			return &key, nil
		}
	}

	return nil, fmt.Errorf("listkeys contained keys, but none of them have full access")
}

func (handler *daprStateStoreAzureStorageHandler) DeleteStorageAccount(ctx context.Context, accountName string) error {
	sc := azclients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	_, err := sc.Delete(ctx, handler.arm.ResourceGroup, accountName)
	if err != nil {
		return fmt.Errorf("failed to delete storage account: %w", err)
	}

	return nil
}

func (handler *daprStateStoreAzureStorageHandler) DeleteDaprStateStore(ctx context.Context, properties map[string]string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ComponentNameKey],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr Component: %w", err)
	}

	return nil
}
