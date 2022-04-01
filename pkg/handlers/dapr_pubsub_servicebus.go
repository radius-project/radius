// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/pkg/resourcemodel"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprPubSubServiceBusHandler(arm *armauth.ArmConfig, k8s client.Client) ResourceHandler {
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

	// This assertion is important so we don't start creating/modifying a resource
	err := ValidateResourceIDsForResource(properties, ServiceBusNamespaceIDKey, ServiceBusTopicIDKey)
	if err != nil {
		return nil, err
	}

	var namespace *servicebus.SBNamespace

	// This is mostly called for the side-effect of verifying that the servicebus namespace exists.
	namespace, err = handler.GetNamespaceByID(ctx, properties[ServiceBusNamespaceIDKey])
	if err != nil {
		return nil, err
	}

	var topic *servicebus.SBTopic

	// This is mostly called for the side-effect of verifying that the servicebus queue exists.
	topic, err = handler.GetTopicByID(ctx, properties[ServiceBusTopicIDKey])
	if err != nil {
		return nil, err
	}

	// Use the identity of the topic as the thing to monitor.
	options.Resource.Identity = resourcemodel.NewARMIdentity(&options.Resource.ResourceType, *topic.ID, clients.GetAPIVersionFromUserAgent(servicebus.UserAgent()))

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

	err := handler.DeleteDaprPubSub(ctx, properties)
	if err != nil {
		return err
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
				"name":      properties[ResourceName],
				"labels":    kubernetes.MakeDescriptiveLabels(options.ApplicationName, options.ResourceName),
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
				"name":      properties[ResourceName],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}

func NewDaprPubSubServiceBusHealthHandler(arm *armauth.ArmConfig, k8s client.Client) HealthHandler {
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
