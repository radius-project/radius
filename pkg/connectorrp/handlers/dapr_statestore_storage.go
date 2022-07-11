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
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/pkg/ucp/resources"
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

func (handler *daprStateStoreAzureStorageHandler) Put(ctx context.Context, resource *outputresource.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("invalid required properties for resource")
	}

	_, ok = properties[ResourceIDKey]
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("missing required property %s for the resource", ResourceIDKey)
	}

	parsedID, err := resources.Parse(properties[ResourceIDKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	sac := clients.NewAccountsClient(parsedID.FindScope(resources.SubscriptionsSegment), handler.arm.Auth)
	account, err := sac.GetProperties(ctx, parsedID.FindScope(resources.ResourceGroupsSegment), properties[StorageAccountNameKey], storage.AccountExpand(""))
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *account.ID, clients.GetAPIVersionFromUserAgent(storage.UserAgent()))

	key, err := handler.findStorageKey(ctx, *account.Name)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = handler.createDaprStateStore(ctx, *account.Name, *key.Value, properties)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	return outputResourceIdentity, properties, nil
}

func (handler *daprStateStoreAzureStorageHandler) Delete(ctx context.Context, resource *outputresource.OutputResource) error {
	properties := resource.Resource.(map[string]string)

	err := handler.deleteDaprStateStore(ctx, properties)
	if err != nil {
		return err
	}

	return nil
}

func (handler *daprStateStoreAzureStorageHandler) createDaprStateStore(ctx context.Context, accountName string, accountKey string, properties map[string]string) error {
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
				"name":      kubernetes.MakeResourceName(properties[ApplicationName], properties[ResourceName]),
				"labels":    kubernetes.MakeDescriptiveLabels(properties[ApplicationName], properties[ResourceName]),
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

func (handler *daprStateStoreAzureStorageHandler) findStorageKey(ctx context.Context, accountName string) (*storage.AccountKey, error) {
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

func (handler *daprStateStoreAzureStorageHandler) deleteDaprStateStore(ctx context.Context, properties map[string]string) error {
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
