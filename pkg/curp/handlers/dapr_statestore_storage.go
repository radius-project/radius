// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/storage/mgmt/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/gofrs/uuid"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StorageAccountBaseNameKey = "storageaccountbasename"
	StorageAccountNameKey     = "storageaccount"
	StorageAccountIDKey       = "storageaccountid"
)

func NewDaprStateStoreAzureStorageHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreAzureStorageHandler{arm: arm, k8s: k8s}
}

type daprStateStoreAzureStorageHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

func (sssh *daprStateStoreAzureStorageHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	sc := storage.NewAccountsClient(sssh.arm.SubscriptionID)
	sc.Authorizer = sssh.arm.Auth

	properties := mergeProperties(options.Resource, options.Existing)
	name, ok := properties[StorageAccountNameKey]
	if !ok {
		// names are kinda finicky here - they have to be unique across azure.
		base := properties[StorageAccountBaseNameKey]
		name = ""

		for i := 0; i < 10; i++ {
			// 3-24 characters - all alphanumeric
			uid, err := uuid.NewV4()
			if err != nil {
				return nil, fmt.Errorf("failed to generate storage account name: %w", err)
			}
			name = base + strings.ReplaceAll(uid.String(), "-", "")
			name = name[0:24]

			result, err := sc.CheckNameAvailability(ctx, storage.AccountCheckNameAvailabilityParameters{
				Name: to.StringPtr(name),
				Type: to.StringPtr("Microsoft.Storage/storageAccounts"),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to query storage account name: %w", err)
			}

			if result.NameAvailable != nil && *result.NameAvailable {
				properties[StorageAccountNameKey] = name
				break
			}

			log.Printf("storage account name generation failed: %v %v", result.Reason, result.Message)
		}
	}

	if name == "" {
		return nil, fmt.Errorf("failed to find a storage name")
	}

	rgc := resources.NewGroupsClient(sssh.arm.SubscriptionID)
	rgc.Authorizer = sssh.arm.Auth

	g, err := rgc.Get(ctx, sssh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	future, err := sc.Create(ctx, sssh.arm.ResourceGroup, name, storage.AccountCreateParameters{
		Location: g.Location,
		Kind:     storage.StorageV2,
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
		},
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
		Tags: map[string]*string{
			radresources.TagRadiusApplication: &options.Application,
			radresources.TagRadiusComponent:   &options.Component,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	account, err := future.Result(sc)
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	// store storage account so we can delete later
	properties[StorageAccountIDKey] = *account.ID

	keys, err := sc.ListKeys(ctx, sssh.arm.ResourceGroup, name, "")
	if err != nil {
		return nil, fmt.Errorf("failed to PUT storage account: %w", err)
	}

	// Since we're doing this programmatically, let's make sure we can find a key with write access.
	if keys.Keys == nil || len(*keys.Keys) == 0 {
		return nil, fmt.Errorf("listkeys returned an empty or nil list of keys")
	}

	// Don't rely on the order the keys are in, we need Full access
	var key *storage.AccountKey
	for _, k := range *keys.Keys {
		if strings.EqualFold(string(k.Permissions), string(storage.Full)) {
			key = &k
			break
		}
	}

	if key == nil {
		return nil, fmt.Errorf("listkeys contained keys, but none of them have full access")
	}

	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[KubernetesNameKey],
			},
			"spec": map[string]interface{}{
				"type":    "state.azure.tablestorage",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "accountName",
						"value": name,
					},
					map[string]interface{}{
						"name":  "accountKey",
						"value": *key.Value,
					},
					map[string]interface{}{
						"name":  "tableName",
						"value": "dapr",
					},
				},
			},
		},
	}

	err = sssh.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (sssh *daprStateStoreAzureStorageHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[KubernetesNameKey],
			},
		},
	}

	err := client.IgnoreNotFound(sssh.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	sc := storage.NewAccountsClient(sssh.arm.SubscriptionID)
	sc.Authorizer = sssh.arm.Auth

	_, err = sc.Delete(ctx, sssh.arm.ResourceGroup, properties[StorageAccountNameKey])
	if err != nil {
		return err
	}

	return nil
}
