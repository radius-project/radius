// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/namegenerator"
)

func NewAzureServiceBusQueueHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureServiceBusQueueHandler{arm: arm}
}

type azureServiceBusQueueHandler struct {
	arm armauth.ArmConfig
}

func (sbh *azureServiceBusQueueHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	sbc := servicebus.NewNamespacesClient(sbh.arm.SubscriptionID)
	sbc.Authorizer = sbh.arm.Auth

	// Check if a service bus namespace exists in the resource group
	sbItr, err := sbc.ListByResourceGroupComplete(ctx, sbh.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("Failed to list service bus namespaces: %w", err)
	}

	var sbNamespace servicebus.SBNamespace
	if sbItr.NotDone() &&
		radresources.HasRadiusApplicationTag(sbItr.Value().Tags, options.Application) {
		// A service bus namespace already exists
		sbNamespace = sbItr.Value()
	} else {
		// Generate a random namespace name
		namespaceName := namegenerator.GenerateName("radius-ns")

		// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
		// in something to do with data locality.
		rgc := resources.NewGroupsClient(sbh.arm.SubscriptionID)
		rgc.Authorizer = sbh.arm.Auth

		g, err := rgc.Get(ctx, sbh.arm.ResourceGroup)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		sbNamespaceFuture, err := sbc.CreateOrUpdate(ctx, sbh.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
			Sku: &servicebus.SBSku{
				Name:     servicebus.Basic,
				Tier:     servicebus.SkuTierBasic,
				Capacity: to.Int32Ptr(1),
			},
			Location: g.Location,
			Tags: map[string]*string{
				radresources.TagRadiusApplication: &options.Application,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}

		sbNamespace, err = sbNamespaceFuture.Result(sbc)
		if err != nil {
			return nil, fmt.Errorf("failed to PUT service bus: %w", err)
		}
	}

	// store account so we can delete later
	properties["servicebusnamespace"] = *sbNamespace.Name
	properties["servicebusid"] = *sbNamespace.ID

	queueName, ok := properties["servicebusqueue"]
	if !ok {
		return nil, fmt.Errorf("failed to PUT service bus: %w", err)
	}
	qc := servicebus.NewQueuesClient(sbh.arm.SubscriptionID)
	qc.Authorizer = sbh.arm.Auth

	sbq, err := qc.CreateOrUpdate(ctx, sbh.arm.ResourceGroup, *sbNamespace.Name, queueName, servicebus.SBQueue{
		Name: to.StringPtr(queueName),
		SBQueueProperties: &servicebus.SBQueueProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to PUT servicebus queue: %w", err)
	}

	// store db so we can delete later
	properties["queueName"] = *sbq.Name
	return properties, nil
}

func (sbh *azureServiceBusQueueHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	namespaceName := properties["servicebusnamespace"]
	queueName := properties["servicebusqueue"]

	qc := servicebus.NewQueuesClient(sbh.arm.SubscriptionID)
	qc.Authorizer = sbh.arm.Auth

	result, err := qc.Delete(ctx, sbh.arm.ResourceGroup, namespaceName, queueName)
	if err != nil && result.StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus queue: %w", err)
	}

	qItr, err := qc.ListByNamespaceComplete(ctx, sbh.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil && qItr.Response().StatusCode != 404 {
		return fmt.Errorf("failed to DELETE servicebus queue: %w", err)
	}

	if qItr.NotDone() {
		// There are other queues in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return nil
	}

	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := servicebus.NewNamespacesClient(sbh.arm.SubscriptionID)
	sbc.Authorizer = sbh.arm.Auth

	sbNamespaceFuture, err := sbc.Delete(ctx, sbh.arm.ResourceGroup, namespaceName)
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
