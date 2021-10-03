// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprPubSubServiceBusHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprPubSubServiceBusHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
		kubernetesHandler:          kubernetesHandler{k8s: k8s},
		k8s:                        k8s,
	}
}

type daprPubSubServiceBusHandler struct {
	azureServiceBusBaseHandler
	kubernetesHandler
	k8s client.Client
}

func (handler *daprPubSubServiceBusHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

	// topic name must be specified by the user
	topicName, ok := properties[ServiceBusTopicNameKey]
	if !ok {
		return nil, fmt.Errorf("missing required property '%s'", ServiceBusTopicIDKey)
	}

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, ServiceBusNamespaceIDKey, ServiceBusTopicIDKey)
	if err != nil {
		return nil, err
	}

	var namespace *servicebus.SBNamespace
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

	var topic *servicebus.SBTopic
	if properties[ServiceBusTopicIDKey] == "" {
		topic, err = handler.CreateTopic(ctx, *namespace.Name, topicName)
		if err != nil {
			return nil, err
		}
		properties[ServiceBusTopicIDKey] = *topic.ID
	} else {
		// This is mostly called for the side-effect of verifying that the servicebus queue exists.
		topic, err = handler.GetTopicByID(ctx, properties[ServiceBusTopicIDKey])
		if err != nil {
			return nil, err
		}
	}

	// Use the identity of the topic as the thing to monitor.
	options.Resource.Identity = resourcemodel.NewARMIdentity(*topic.ID, clients.GetAPIVersionFromUserAgent(servicebus.UserAgent()))

	cs, err := handler.GetConnectionString(ctx, *namespace.Name)
	if err != nil {
		return nil, err
	}

	err = handler.PatchDaprPubSub(ctx, properties, *cs, *options)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (handler *daprPubSubServiceBusHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

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

func (handler *daprPubSubServiceBusHandler) PatchDaprPubSub(ctx context.Context, properties map[string]string, cs string, options PutOptions) error {
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
				"labels":    kubernetes.MakeDescriptiveLabels(options.Application, options.Component),
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

	err = handler.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: kubernetes.FieldManager})
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
				"name":      properties[ComponentNameKey],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}

func NewDaprPubSubServiceBusHealthHandler(arm armauth.ArmConfig, k8s client.Client) HealthHandler {
	return &daprPubSubServiceBusHealthHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
		kubernetesHandler:          kubernetesHandler{k8s: k8s},
		k8s:                        k8s,
	}
}

type daprPubSubServiceBusHealthHandler struct {
	azureServiceBusBaseHandler
	kubernetesHandler
	k8s client.Client
}

func (handler *daprPubSubServiceBusHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
