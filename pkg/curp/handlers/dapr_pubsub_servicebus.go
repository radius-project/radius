// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/curp/armauth"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprPubSubServiceBusHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprPubSubServiceBusHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
		k8s:                        k8s,
	}
}

type daprPubSubServiceBusHandler struct {
	azureServiceBusBaseHandler
	k8s client.Client
}

func (handler *daprPubSubServiceBusHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// topic name must be specified by the user
	topicName, ok := properties[ServiceBusTopicNameKey]
	if !ok {
		return nil, fmt.Errorf("missing required property '%s'", ServiceBusTopicIDKey)
	}

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	if properties[ManagedKey] != "true" && (properties[ServiceBusNamespaceIDKey] == "" || properties[ServiceBusTopicIDKey] == "") {
		return nil, fmt.Errorf("missing required properties '%s' and '%s' for an unmanaged resource", ServiceBusNamespaceIDKey, ServiceBusTopicIDKey)
	}

	var namespace *servicebus.SBNamespace
	var err error
	if properties[ServiceBusNamespaceIDKey] == "" {
		// If we don't have an ID already then we will need to create a new one.
		namespace, err = handler.LookupSharedManagedNamespaceFromResourceGroup(ctx, options.Application)
		if err != nil {
			return nil, err
		}

		if namespace == nil {
			namespace, err = handler.CreateNamespace(ctx, options.Application)
			if err != nil {
				return nil, err
			}
		}

		properties[ServiceBusNamespaceNameKey] = *namespace.Name
		properties[ServiceBusNamespaceIDKey] = *namespace.ID
	} else {
		// This is mostly called for the side-effect of verifying that the servicebus namespace exists.
		namespace, err = handler.GetNamespaceByID(ctx, properties[ServiceBusNamespaceIDKey])
		if err != nil {
			return nil, err
		}
	}

	if properties[ServiceBusTopicIDKey] == "" {
		queue, err := handler.CreateTopic(ctx, *namespace.Name, topicName)
		if err != nil {
			return nil, err
		}
		properties[ServiceBusTopicIDKey] = *queue.ID
	} else {
		// This is mostly called for the side-effect of verifying that the servicebus queue exists.
		_, err := handler.GetTopicByID(ctx, properties[ServiceBusTopicIDKey])
		if err != nil {
			return nil, err
		}
	}

	cs, err := handler.GetConnectionString(ctx, *namespace.Name)
	if err != nil {
		return nil, err
	}

	err = handler.PatchDaprPubSub(ctx, properties, *cs)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprPubSubServiceBusHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	namespaceName := properties[ServiceBusNamespaceNameKey]
	topicName := properties[ServiceBusTopicNameKey]

	err := handler.DeleteDaprPubSub(ctx, properties)
	if err != nil {
		return err
	}

	if properties[ManagedKey] == "true" {
		deleteNamespace, err := handler.DeleteTopic(ctx, namespaceName, topicName)
		if err != nil {
			return err
		}

		if deleteNamespace {
			err = handler.DeleteNamespace(ctx, namespaceName)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (handler *daprPubSubServiceBusHandler) PatchDaprPubSub(ctx context.Context, properties map[string]string, cs string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties[KubernetesAPIVersionKey],
			"kind":       properties[KubernetesKindKey],
			"metadata": map[string]interface{}{
				"namespace": properties[KubernetesNamespaceKey],
				"name":      properties[KubernetesNameKey],
			},
			"spec": map[string]interface{}{
				"type":    "pubsub.azure.servicebus",
				"version": "v1",
				"metadata": []interface{}{
					map[string]interface{}{
						"name":  "connectionString",
						"value": cs,
					},
				},
			},
		},
	}

	err := handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return fmt.Errorf("failed to patch Dapr PubSub: %w", err)
	}

	return nil
}

func (handler *daprPubSubServiceBusHandler) DeleteDaprPubSub(ctx context.Context, properties map[string]string) error {
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

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}
