// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clientv2"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/linkrp"
	"github.com/project-radius/radius/pkg/resourcemodel"
	rpv1 "github.com/project-radius/radius/pkg/rp/v1"
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
		daprComponentHandler: daprComponentHandler{
			k8s: k8s,
		},
		arm: arm,
		k8s: k8s,
	}
}

type daprStateStoreAzureStorageHandler struct {
	daprComponentHandler
	arm *armauth.ArmConfig
	k8s client.Client
}

func (handler *daprStateStoreAzureStorageHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("invalid required properties for resource")
	}

	id, ok := properties[ResourceIDKey]
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("missing required property %s for the resource", ResourceIDKey)
	}

	parsedID, err := resources.ParseResource(id)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to parse Storage Account resource id: %w", err)
	}

	client, err := clientv2.NewAccountsClient(parsedID.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOptions)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to create Storage Account client: %w", err)
	}

	account, err := client.GetProperties(ctx, parsedID.FindScope(resources.ResourceGroupsSegment), properties[StorageAccountNameKey], nil)
	if err != nil {
		if clientv2.Is404Error(err) {
			return resourcemodel.ResourceIdentity{}, nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure Storage Account %q does not exist", properties[StorageAccountNameKey]))
		}
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("failed to get Storage Account: %w", err)
	}

	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *account.ID, clientv2.AccountsClientAPIVersion)

	key, err := handler.findStorageKey(ctx, *account.ID)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = checkResourceNameUniqueness(ctx, handler.k8s, kubernetes.NormalizeResourceName(properties[ResourceName]), properties[KubernetesNamespaceKey], linkrp.DaprStateStoresResourceType)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = handler.createDaprStateStore(ctx, *account.Name, *key.Value, properties)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	return outputResourceIdentity, properties, nil
}

func (handler *daprStateStoreAzureStorageHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	properties := resource.Resource.(map[string]any)

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
		Object: map[string]any{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]any{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      kubernetes.NormalizeResourceName(properties[ResourceName]),
				"labels":    kubernetes.MakeDescriptiveLabels(properties[ApplicationName], properties[ResourceName], linkrp.DaprStateStoresResourceType),
			},
			"spec": map[string]any{
				"type":    "state.azure.tablestorage",
				"version": "v1",
				"metadata": []any{
					map[string]any{
						"name":  "accountName",
						"value": accountName,
					},
					map[string]any{
						"name":  "accountKey",
						"value": accountKey,
					},
					map[string]any{
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

func (handler *daprStateStoreAzureStorageHandler) findStorageKey(ctx context.Context, id string) (*armstorage.AccountKey, error) {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	client, err := clientv2.NewAccountsClient(parsed.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Storage Account client: %w", err)
	}

	resp, err := client.ListKeys(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.Name(), &armstorage.AccountsClientListKeysOptions{
		Expand: nil,
	})
	if err != nil {
		if clientv2.Is404Error(err) {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure Storage Account %q does not exist", parsed.Name()))
		}
		return nil, fmt.Errorf("failed to access keys of storage account: %w", err)
	}

	// Since we're doing this programmatically, let's make sure we can find a key with write access.
	if resp.Keys == nil || len(resp.Keys) == 0 {
		return nil, fmt.Errorf("listkeys returned an empty or nil list of keys")
	}

	// Don't rely on the order the keys are in, we need Full access
	for _, key := range resp.Keys {
		if strings.EqualFold(string(*key.Permissions), string(armstorage.KeyPermissionFull)) {
			return key, nil
		}
	}

	return nil, fmt.Errorf("listkeys contained keys, but none of them have full access")
}

func (handler *daprStateStoreAzureStorageHandler) deleteDaprStateStore(ctx context.Context, properties map[string]any) error {
	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]any{
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
