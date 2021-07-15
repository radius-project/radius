// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/to"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/rad/namegenerator"
	"github.com/Azure/radius/pkg/rad/util"
	"github.com/Azure/radius/pkg/radrp/armauth"
	radresources "github.com/Azure/radius/pkg/radrp/resources"
)

const (
	ServiceBusNamespaceIDKey   = "servicebusid"
	ServiceBusNamespaceNameKey = "servicebusnamespace"
	ServiceBusQueueNameKey     = "servicebusqueue"
	ServiceBusQueueIDKey       = "servicebusqueueid"
	ServiceBusTopicNameKey     = "servicebustopic"
	ServiceBusTopicIDKey       = "servicebustopicid"
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

func (handler *azureServiceBusQueueHandler) Put(ctx context.Context, options PutOptions) (map[string]string, error) {
	properties := mergeProperties(options.Resource, options.Existing)

	// queue name must be specified by the user
	queueName, ok := properties[ServiceBusQueueNameKey]
	if !ok {
		return nil, fmt.Errorf("missing required property '%s'", ServiceBusQueueNameKey)
	}

	// This assertion is important so we don't start creating/modifying an unmanaged resource
	err := ValidateResourceIDsForUnmanagedResource(properties, ServiceBusNamespaceIDKey, ServiceBusQueueIDKey)
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

	if properties[ServiceBusQueueIDKey] == "" {
		queue, err := handler.CreateQueue(ctx, *namespace.Name, queueName)
		if err != nil {
			return nil, err
		}
		properties[ServiceBusQueueIDKey] = *queue.ID
	} else {
		// This is mostly called for the side-effect of verifying that the servicebus queue exists.
		_, err := handler.GetQueueByID(ctx, properties[ServiceBusQueueIDKey])
		if err != nil {
			return nil, err
		}
	}

	return properties, nil
}

func (handler *azureServiceBusQueueHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.Existing.Properties
	if properties[ManagedKey] != "true" {
		// For an 'unmanaged' resource we don't need to do anything, just forget it.
		return nil
	}

	namespaceName := properties[ServiceBusNamespaceNameKey]
	queueName := properties[ServiceBusQueueNameKey]

	deleteNamespace, err := handler.DeleteQueue(ctx, namespaceName, queueName)
	if err != nil {
		return err
	}

	if deleteNamespace {
		// The last queue in the service bus namespace was deleted. Now delete the namespace as well
		err = handler.DeleteNamespace(ctx, namespaceName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (handler *azureServiceBusBaseHandler) GetNamespaceByID(ctx context.Context, id string) (*servicebus.SBNamespace, error) {
	parsed, err := radresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus queue resource id: %w", err)
	}

	sbc := servicebus.NewNamespacesClient(parsed.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth
	sbc.PollingDuration = 0

	// Check if a service bus namespace exists in the resource group for this application
	namespace, err := sbc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus namespace: %w", err)
	}

	return &namespace, nil
}

func (handler *azureServiceBusBaseHandler) LookupSharedManagedNamespaceFromResourceGroup(ctx context.Context, application string) (*servicebus.SBNamespace, error) {
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth
	sbc.PollingDuration = 0

	// Check if a service bus namespace exists in the resource group for this application
	list, err := sbc.ListByResourceGroupComplete(ctx, handler.arm.ResourceGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to list service bus namespaces: %w", err)
	}

	// Azure Service Bus needs StandardTier or higher SKU to support topics
	if list.NotDone() &&
		list.Value().Sku.Tier != servicebus.SkuTierBasic &&
		keys.HasRadiusApplicationTag(list.Value().Tags, application) {
		// A service bus namespace already exists
		namespace := list.Value()
		return &namespace, nil
	}

	return nil, nil
}

func (handler *azureServiceBusBaseHandler) CreateNamespace(ctx context.Context, application string) (*servicebus.SBNamespace, error) {
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth
	sbc.PollingDuration = 0

	location, err := getResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	// Generate a random namespace name
	namespaceName := namegenerator.GenerateName("radius-ns")

	future, err := sbc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
		Sku: &servicebus.SBSku{
			Name:     servicebus.Standard,
			Tier:     servicebus.SkuTierStandard,
			Capacity: to.Int32Ptr(1),
		},
		Location: location,

		// NOTE: this is a special case, we currently share servicebus resources per-application
		// they are not directly associated with a component. See: #176
		Tags: map[string]*string{
			keys.TagRadiusApplication: &application,
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
	tc.PollingDuration = 0

	topic, err := tc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, topicName, servicebus.SBTopic{
		Name: to.StringPtr(topicName),
		SBTopicProperties: &servicebus.SBTopicProperties{
			MaxSizeInMegabytes: to.Int32Ptr(1024),
		},

		// NOTE: Service bus topics don't support tags
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create servicebus topic: %w", err)
	}

	return &topic, nil
}

func (handler *azureServiceBusBaseHandler) GetTopicByID(ctx context.Context, id string) (*servicebus.SBTopic, error) {
	parsed, err := radresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus resource id: %w", err)
	}

	tc := servicebus.NewTopicsClient(parsed.SubscriptionID)
	tc.Authorizer = handler.arm.Auth
	tc.PollingDuration = 0

	topic, err := tc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus queue: %w", err)
	}

	return &topic, nil
}

func (handler *azureServiceBusBaseHandler) CreateQueue(ctx context.Context, namespaceName string, queueName string) (*servicebus.SBQueue, error) {
	qc := servicebus.NewQueuesClient(handler.arm.SubscriptionID)
	qc.Authorizer = handler.arm.Auth
	qc.PollingDuration = 0

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

func (handler *azureServiceBusBaseHandler) GetQueueByID(ctx context.Context, id string) (*servicebus.SBQueue, error) {
	parsed, err := radresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus resource id: %w", err)
	}

	qc := servicebus.NewQueuesClient(parsed.ID)
	qc.Authorizer = handler.arm.Auth
	qc.PollingDuration = 0

	queue, err := qc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus queue: %w", err)
	}

	return &queue, nil
}

func (handler *azureServiceBusBaseHandler) GetConnectionString(ctx context.Context, namespaceName string) (*string, error) {
	sbc := servicebus.NewNamespacesClient(handler.arm.SubscriptionID)
	sbc.Authorizer = handler.arm.Auth
	sbc.PollingDuration = 0

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
	sbc.PollingDuration = 0

	sbNamespaceFuture, err := sbc.Delete(ctx, handler.arm.ResourceGroup, namespaceName)
	if err != nil && sbNamespaceFuture.Response().StatusCode != 404 {
		return fmt.Errorf("failed to delete servicebus namespace: %w", err)
	}

	err = sbNamespaceFuture.WaitForCompletionRef(ctx, sbc.Client)
	if err != nil && !util.IsAutorest404Error(err) {
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
	tc.PollingDuration = 0

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
	qc.PollingDuration = 0

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
