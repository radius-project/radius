// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"
	"strings"

	armservicebus "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/servicebus/armservicebus/v2"
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
	ServiceBusNamespaceIDKey   = "servicebusid"
	RootManageSharedAccessKey  = "RootManageSharedAccessKey"
	ServiceBusTopicNameKey     = "servicebustopic"
	ServiceBusNamespaceNameKey = "servicebusnamespace"
)

type daprPubSubServiceBusBaseHandler struct {
	arm *armauth.ArmConfig
}

type daprPubSubServiceBusHandler struct {
	daprPubSubServiceBusBaseHandler
	daprComponentHandler
	k8s client.Client
}

func NewDaprPubSubServiceBusHandler(arm *armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprPubSubServiceBusHandler{
		daprPubSubServiceBusBaseHandler: daprPubSubServiceBusBaseHandler{arm: arm},
		daprComponentHandler: daprComponentHandler{
			k8s: k8s,
		},
		k8s: k8s,
	}
}

func (handler *daprPubSubServiceBusHandler) Put(ctx context.Context, resource *rpv1.OutputResource) (outputResourceIdentity resourcemodel.ResourceIdentity, properties map[string]string, err error) {
	properties, ok := resource.Resource.(map[string]string)
	if !ok {
		return resourcemodel.ResourceIdentity{}, nil, fmt.Errorf("invalid required properties for resource")
	}

	// This assertion is important so we don't start creating/modifying a resource
	err = ValidateResourceIDsForResource(properties, ServiceBusNamespaceIDKey)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	var namespace *armservicebus.SBNamespace

	// This is mostly called for the side-effect of verifying that the servicebus namespace exists.
	namespace, err = handler.GetNamespaceByID(ctx, properties[ServiceBusNamespaceIDKey])
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	// Use the identity of the namespace as the thing to monitor.
	outputResourceIdentity = resourcemodel.NewARMIdentity(&resource.ResourceType, *namespace.ID, clientv2.ServiceBusClientAPIVersion)

	cs, err := handler.GetConnectionString(ctx, *namespace.ID)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = checkResourceNameUniqueness(ctx, handler.k8s, kubernetes.NormalizeResourceName(properties[ResourceName]), properties[KubernetesNamespaceKey], linkrp.DaprPubSubBrokersResourceType)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	err = handler.PatchDaprPubSub(ctx, properties, *cs, resource)
	if err != nil {
		return resourcemodel.ResourceIdentity{}, nil, err
	}

	return outputResourceIdentity, properties, nil
}

func (handler *daprPubSubServiceBusHandler) Delete(ctx context.Context, resource *rpv1.OutputResource) error {
	properties := resource.Resource.(map[string]any)

	fmt.Printf("Deleting Dapr service bus component %s\n", resource.Identity.GetID())

	err := handler.DeleteDaprPubSub(ctx, properties)
	if err != nil {
		return err
	}

	return nil
}

func (handler *daprPubSubServiceBusHandler) PatchDaprPubSub(ctx context.Context, properties map[string]string, cs string, resource *rpv1.OutputResource) error {
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
				"labels":    kubernetes.MakeDescriptiveLabels(properties[ApplicationName], properties[ResourceName], linkrp.DaprPubSubBrokersResourceType),
			},
			"spec": map[string]any{
				"type":    "pubsub.azure.servicebus",
				"version": "v1",
				"metadata": []any{
					map[string]any{
						"name":  "connectionString",
						"value": cs,
					},
				},
			},
		},
	}

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
	if err != nil {
		return fmt.Errorf("failed to patch Dapr PubSub: %w", err)
	}

	return nil
}

func (handler *daprPubSubServiceBusHandler) DeleteDaprPubSub(ctx context.Context, properties map[string]any) error {
	item := unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]any{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      kubernetes.NormalizeResourceName(properties[ResourceName].(string)),
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}

func (handler *daprPubSubServiceBusBaseHandler) GetNamespaceByID(ctx context.Context, id string) (*armservicebus.SBNamespace, error) {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus queue resource id: '%s':%w", id, err)
	}

	client, err := clientv2.NewServiceBusNamespacesClient(parsed.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus namespace client:%w", err)
	}

	// Check if a service bus namespace exists in the resource group for this application
	resp, err := client.Get(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.TypeSegments()[0].Name, nil)
	if err != nil {
		if clientv2.Is404Error(err) {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure ServiceBus Namespace %q does not exist", id))
		}
		return nil, fmt.Errorf("failed to get servicebus namespace:%w", err)
	}

	return &resp.SBNamespace, nil
}

func (handler *daprPubSubServiceBusBaseHandler) GetConnectionString(ctx context.Context, id string) (*string, error) {
	parsed, err := resources.ParseResource(id)
	if err != nil {
		return nil, err
	}

	client, err := clientv2.NewServiceBusNamespacesClient(parsed.FindScope(resources.SubscriptionsSegment), &handler.arm.ClientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus namespace client:%w", err)
	}

	accessKeys, err := client.ListKeys(ctx, parsed.FindScope(resources.ResourceGroupsSegment), parsed.Name(), RootManageSharedAccessKey, nil)
	if err != nil {
		if clientv2.Is404Error(err) {
			return nil, v1.NewClientErrInvalidRequest(fmt.Sprintf("provided Azure ServiceBus Namespace %q does not exist", parsed.Name()))
		}
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	return accessKeys.PrimaryConnectionString, nil
}

func ValidateResourceIDsForResource(properties map[string]string, keys ...string) error {
	missing := []string{}
	for _, k := range keys {
		_, ok := properties[k]
		if !ok {
			// Surround with single-quotes for formatting later
			missing = append(missing, fmt.Sprintf("'%s'", k))
		}
	}

	if len(missing) == 0 {
		return nil
	}

	return fmt.Errorf("missing required properties %v for resource", strings.Join(missing, ", "))
}
