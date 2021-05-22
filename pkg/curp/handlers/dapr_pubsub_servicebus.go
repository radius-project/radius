// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	azresources "github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/namegenerator"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewDaprPubSubServiceBusHandler(arm armauth.ArmConfig, k8s client.Client) ResourceHandler {
	return &daprPubSubServiceBusHandler{arm: arm, k8s: k8s}
}

type daprPubSubServiceBusHandler struct {
	arm armauth.ArmConfig
	k8s client.Client
}

func (pssb *daprPubSubServiceBusHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	sbc := servicebus.NewNamespacesClient(pssb.arm.SubscriptionID)
	sbc.Authorizer = pssb.arm.Auth

	// Check if a service bus namespace exists in the resource group for this application
	sbItr, err := sbc.ListByResourceGroupComplete(ctx, pssb.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("Failed to list service bus namespaces: %w", err)
	}

	var sbNamespace servicebus.SBNamespace

	// Azure Service Bus needs StandardTier or higher SKU to support topics
	if sbItr.NotDone() &&
		sbItr.Value().Sku.Tier != servicebus.SkuTierBasic &&
		radresources.HasRadiusApplicationTag(sbItr.Value().Tags, options.Application) {
		// A service bus namespace already exists
		sbNamespace = sbItr.Value()
	} else {
		// Generate a random namespace name
		namespaceName := namegenerator.GenerateName("radius-ns")

		// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
		// in something to do with data locality.
		rgc := azresources.NewGroupsClient(pssb.arm.SubscriptionID)
		rgc.Authorizer = pssb.arm.Auth

		g, err := rgc.Get(ctx, pssb.arm.ResourceGroup)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		sbNamespaceFuture, err := sbc.CreateOrUpdate(ctx, pssb.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
			Sku: &servicebus.SBSku{
				Name:     servicebus.Standard,
				Tier:     servicebus.SkuTierStandard,
				Capacity: to.Int32Ptr(1),
			},
			Location: g.Location,
			Tags: map[string]*string{
				radresources.TagRadiusApplication: &options.Application,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}

		sbNamespace, err = sbNamespaceFuture.Result(sbc)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
		}
	}

	properties["servicebusnamespace"] = *sbNamespace.Name
	properties["servicebusid"] = *sbNamespace.ID

	topicName, ok := properties["servicebustopic"]
	if !ok {
		return nil, fmt.Errorf("failed to PUT service bus pubsub: %w", err)
	}
	tc := servicebus.NewTopicsClient(pssb.arm.SubscriptionID)
	tc.Authorizer = pssb.arm.Auth

	sbTopic, err := tc.CreateOrUpdate(ctx, pssb.arm.ResourceGroup, *sbNamespace.Name, topicName, servicebus.SBTopic{
		Name: to.StringPtr(topicName),
		SBTopicProperties: &servicebus.SBTopicProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT servicebus topic: %w", err)
	}

	// store db so we can delete later
	properties["topicName"] = *sbTopic.Name

	accessKeys, err := sbc.ListKeys(ctx, pssb.arm.ResourceGroup, *sbNamespace.Name, "RootManageSharedAccessKey")

	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil && accessKeys.SecondaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	cs := accessKeys.PrimaryConnectionString

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

	err = pssb.k8s.Patch(ctx, &item, client.Apply, &client.PatchOptions{FieldManager: "radius-rp"})
	if err != nil {
		return nil, err
	}

	return properties, nil
}

func (pssb *daprPubSubServiceBusHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
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

	err := client.IgnoreNotFound(pssb.k8s.Delete(ctx, &item))
	if err != nil {
		return err
	}

	namespaceName := properties["servicebusnamespace"]
	topicName := properties["servicebustopic"]

	tc := servicebus.NewTopicsClient(pssb.arm.SubscriptionID)
	tc.Authorizer = pssb.arm.Auth

	result, err := tc.Delete(ctx, pssb.arm.ResourceGroup, namespaceName, topicName)
	if err != nil && result.StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	tItr, err := tc.ListByNamespaceComplete(ctx, pssb.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil && tItr.Response().StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	// Delete service bus topic only marks the topic for deletion but does not actually delete it. Hence the additional check...
	// https://docs.microsoft.com/en-us/rest/api/servicebus/delete-topic
	if tItr.NotDone() && tItr.Value().Name != &topicName {
		// There are other topics in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return nil
	}

	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := servicebus.NewNamespacesClient(pssb.arm.SubscriptionID)
	sbc.Authorizer = pssb.arm.Auth

	sbNamespaceFuture, err := sbc.Delete(ctx, pssb.arm.ResourceGroup, namespaceName)
	if err != nil && sbNamespaceFuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	response, err := sbNamespaceFuture.Result(sbc)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus: %w", err)
	}

	return nil
}
