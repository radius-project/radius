// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

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

func (pssb *daprPubSubServiceBusHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// 'servicbustopic' is a name that must be specified by the user
	topicName, ok := properties["servicebustopic"]
	if !ok {
		return nil, errors.New("missing required property 'servicebustopic'")
	}

	namespace, err := pssb.GetExistingNamespaceFromResourceGroup(ctx, options.Application)
	if err != nil {
		return nil, err
	}

	if namespace == nil {
		namespace, err = pssb.CreateNamespace(ctx, options.Application)
	}

	properties["servicebusnamespace"] = *namespace.Name
	properties["servicebusid"] = *namespace.ID

	_, err = pssb.CreateTopic(ctx, *namespace.Name, topicName)
	if err != nil {
		return nil, err
	}

	cs, err := pssb.GetConnectionString(ctx, *namespace.Name)
	if err != nil {
		return nil, err
	}

	err = pssb.PatchDaprPubSub(ctx, properties, *cs)
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (pssb *daprPubSubServiceBusHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	namespaceName := properties["servicebusnamespace"]
	topicName := properties["servicebustopic"]

	err := pssb.DeleteDaprPubSub(ctx, properties)
	if err != nil {
		return err
	}

	deleteNamespace, err := pssb.DeleteTopic(ctx, namespaceName, topicName)
	if err != nil {
		return err
	}

	if deleteNamespace {
		err = pssb.DeleteNamespace(ctx, namespaceName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (handler *daprPubSubServiceBusHandler) PatchDaprPubSub(ctx context.Context, properties map[string]string, cs string) error {
	item := unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
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
			"apiVersion": properties["apiVersion"],
			"kind":       properties["kind"],
			"metadata": map[string]interface{}{
				"namespace": properties["namespace"],
				"name":      properties["name"],
			},
		},
	}

	err := client.IgnoreNotFound(handler.k8s.Delete(ctx, &item))
	if err != nil {
		return fmt.Errorf("failed to delete Dapr PubSub: %w", err)
	}

	return nil
}
