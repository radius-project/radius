// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"errors"
	"fmt"

	azresources "github.com/Azure/azure-sdk-for-go/profiles/latest/resources/mgmt/resources"
	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/curp/armauth"
	radresources "github.com/Azure/radius/pkg/curp/resources"
	"github.com/Azure/radius/pkg/rad/namegenerator"
)

func NewAzureServiceBusQueueHandler(arm armauth.ArmConfig) ResourceHandler {
	return &azureServiceBusQueueHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
	}
}

type azureServiceBusBaseHandler struct {
	arm armauth.ArmConfig
}

type azureServiceBusQueueHandler struct {
	azureServiceBusBaseHandler
}

func (sbh *azureServiceBusQueueHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// 'servicebusqueue' is a name that must be specified by the user
	queueName, ok := properties["servicebusqueue"]
	if !ok {
		return nil, errors.New("missing required property 'servicebusqueue'")
	}

	namespace, err := sbh.GetExistingNamespaceFromResourceGroup(ctx, options.Application)
	if err != nil {
		return nil, err
	}

	if namespace == nil {
		namespace, err = sbh.CreateNamespace(ctx, options.Application)
	}

	properties["servicebusnamespace"] = *namespace.Name
	properties["servicebusid"] = *namespace.ID

	_, err = sbh.CreateQueue(ctx, *namespace.Name, queueName)
	if err != nil {
		return nil, err
	}

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

func (handler *azureServiceBusBaseHandler) CreateNamespace(ctx context.Context, application string) (*servicebus.SBNamespace, error) {
	rgc := azresources.NewGroupsClient(handler.arm.SubscriptionID)
	rgc.Authorizer = handler.arm.Auth

	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth

	// TODO: for now we just use the resource-groups location. This would be a place where we'd plug
	// in something to do with data locality.
	g, err := rgc.Get(ctx, handler.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group: %w", err)
	}

	// Generate a random namespace name
	namespaceName := namegenerator.GenerateName("radius-ns")

	future, err := sbc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
		Sku: &servicebus.SBSku{
			Name:     servicebus.Standard,
			Tier:     servicebus.SkuTierStandard,
			Capacity: to.Int32Ptr(1),
		},
		Location: g.Location,
		Tags: map[string]*string{
			radresources.TagRadiusApplication: &application,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus namespace: %w", err)
	}

	err = future.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus namespace: %w", err)
	}

	namespace, err := future.Result(sbc)
	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus namespace: %w", err)
	}

	return &namespace, err
}

func (handler *azureServiceBusBaseHandler) CreateTopic(ctx context.Context, namespaceName string, topicName string) (*servicebus.SBTopic, error) {
	tc := servicebus.NewTopicsClient(handler.arm.SubscriptionID)
	tc.Authorizer = handler.arm.Auth

	topic, err := tc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, topicName, servicebus.SBTopic{
		Name: to.StringPtr(topicName),
		SBTopicProperties: &servicebus.SBTopicProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus topic: %w", err)
	}

	return &topic, nil
}

func (handler *azureServiceBusBaseHandler) CreateQueue(ctx context.Context, namespaceName string, queueName string) (*servicebus.SBQueue, error) {
	qc := servicebus.NewQueuesClient(handler.arm.SubscriptionID)
	qc.Authorizer = handler.arm.Auth

	queue, err := qc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, queueName, servicebus.SBQueue{
		Name: to.StringPtr(queueName),
		SBQueueProperties: &servicebus.SBQueueProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus queue: %w", err)
	}

	return &queue, nil
}

func (handler *azureServiceBusBaseHandler) GetConnectionString(ctx context.Context, namespaceName string) (*string, error) {
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth

	accessKeys, err := sbc.ListKeys(ctx, handler.arm.ResourceGroup, namespaceName, "RootManageSharedAccessKey")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	return accessKeys.PrimaryConnectionString, nil
}

func (handler *azureServiceBusBaseHandler) DeleteNamespace(ctx context.Context, namespaceName string) error {
	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth

	sbNamespaceFuture, err := sbc.Delete(ctx, handler.arm.ResourceGroup, namespaceName)
	if err != nil && sbNamespaceFuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete servicebus namespace: %w", err)
	}

	err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil {
		return fmt.Errorf("failed to delete servicebus namespace: %w", err)
	}

	response, err := sbNamespaceFuture.Result(sbc)
	if err != nil && response.StatusCode != 404 {
		return fmt.Errorf("failed to delete servicebus namespace: %w", err)
	}

	return nil
}

// Returns true if the namespace can be deleted
func (handler *azureServiceBusBaseHandler) DeleteTopic(ctx context.Context, namespaceName string, topicName string) (bool, error) {
	tc := servicebus.NewTopicsClient(handler.arm.SubscriptionID)
	tc.Authorizer = handler.arm.Auth

	// We might see a 404 here due the namespace already being deleted. This is benign and could occur on retry
	// of a failed deletion. Either way if the namespace is gone then the topic is gone.
	result, err := tc.Delete(ctx, handler.arm.ResourceGroup, namespaceName, topicName)
	if err != nil && result.StatusCode != 404 {
		return false, fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	tItr, err := tc.ListByNamespaceComplete(ctx, handler.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil && tItr.Response().StatusCode != 404 {
		return false, fmt.Errorf("failed to DELETE servicebus topic: %w", err)
	}

	// Delete service bus topic only marks the topic for deletion but does not actually delete it. Hence the additional check...
	// https://docs.microsoft.com/en-us/rest/api/servicebus/delete-topic
	if tItr.NotDone() && tItr.Value().Name != &topicName {
		// There are other topics in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return false, nil
	}

	// Namespace is empty, it can be deleted if it is unused
	return true, nil
}

// Returns true if the namespace can be deleted
func (handler *azureServiceBusBaseHandler) DeleteQueue(ctx context.Context, namespaceName, queueName string) (bool, error) {
	qc := servicebus.NewQueuesClient(handler.arm.SubscriptionID)
	qc.Authorizer = handler.arm.Auth

	result, err := qc.Delete(ctx, handler.arm.ResourceGroup, namespaceName, queueName)
	if err != nil && result.StatusCode != 404 {
		return false, fmt.Errorf("failed to delete servicebus queue: %w", err)
	}

	qItr, err := qc.ListByNamespaceComplete(ctx, handler.arm.ResourceGroup, namespaceName, nil, nil)
	if err != nil && qItr.Response().StatusCode != 404 {
		return false, fmt.Errorf("failed to delete servicebus queue: %w", err)
	}

	if qItr.NotDone() {
		// There are other queues in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return false, nil
	}

	// Namespace is empty, it can be deleted if it is unused
	return true, nil
}

func (handler *azureServiceBusBaseHandler) GetExistingNamespaceFromResourceGroup(ctx context.Context, application string) (*servicebus.SBNamespace, error) {
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth

	// Check if a service bus namespace exists in the resource group for this application
	list, err := sbc.ListByResourceGroupComplete(ctx, handler.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("Failed to list service bus namespaces: %w", err)
	}

	// Azure Service Bus needs StandardTier or higher SKU to support topics
	if list.NotDone() &&
		list.Value().Sku.Tier != servicebus.SkuTierBasic &&
		radresources.HasRadiusApplicationTag(list.Value().Tags, application) {
		// A service bus namespace already exists
		namespace := list.Value()
		return &namespace, nil
	}

	return nil, nil
}
