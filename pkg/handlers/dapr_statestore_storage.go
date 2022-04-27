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
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	StorageAccountNameKey = "storageaccount"
	ResourceIDKey         = "resourceid"
)

func NewDaprStateStoreAzureStorageHandler(arm *armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprStateStoreAzureStorageHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreAzureStorageHandler struct {
	kubernetesHandler
	arm *armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreAzureStorageHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, ResourceIDKey)
	if err != nil {
		return nil, err
	}

	var account *storage.Account

	account, err = getStorageAccountByID(ctx, *handler.arm, properties[ResourceIDKey])
	if err != nil {
		return nil, err
	}

	// Use the identity of the table storage as the thing to monitor
	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, *account.ID, clients.GetAPIVersionFromUserAgent(storage.UserAgent()))

	key, err := handler.FindStorageKey(ctx, *account.Name)
	if err != nil {
		return nil, err
	}

	err = handler.CreateDaprStateStore(ctx, *account.Name, *key.Value, properties, *options)
	//Nithya : resource name might not be empty here.
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprStateStoreAzureStorageHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

	err := handler.DeleteDaprStateStore(ctx, properties)
	if err != nil {
		return err
	}

	return nil
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
				"name":      properties[ResourceName],
				"labels":    kubernetes.MakeDescriptiveLabels(options.ApplicationName, options.ResourceName),
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

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return fmt.Errorf("failed to create/update Dapr State Store: %w", err)
	}

	return err
}

func (handler *daprStateStoreAzureStorageHandler) FindStorageKey(ctx context.Context, accountName string) (*storage.AccountKey, error) {
	sc := clients.NewAccountsClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
		if strings.EqualFold(string(k.Permissions), string(storage.KeyPermissionFull)) {
			key := k
			return &key, nil
		}
	}

	return nil, fmt.Errorf("listkeys contained keys, but none of them have full access")
}

func (handler *daprStateStoreAzureStorageHandler) DeleteDaprStateStore(ctx context.Context, properties map[string]string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[ResourceName],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr state store: %w", err)
	}

	return nil
}

func NewDaprStateStoreAzureStorageHealthHandler(arm *armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprStateStoreAzureStorageHealthHandler{
		kubernetesHandler: kubernetesHandler{k8s: k8s},
		arm:               arm,
		k8s:               k8s,
	}
}

type daprStateStoreAzureStorageHealthHandler struct {
	kubernetesHandler
	arm *armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreAzureStorageHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
