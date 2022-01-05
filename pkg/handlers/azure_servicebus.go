// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/radius/pkg/azure/armauth"
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/azure/clients"
	"github.com/Azure/radius/pkg/healthcontract"
	"github.com/Azure/radius/pkg/keys"
	"github.com/Azure/radius/pkg/radlogger"
	"github.com/Azure/radius/pkg/resourcemodel"
)

const (
	ServiceBusNamespaceIDKey               = "servicebusid"
	ServiceBusNamespaceNameKey             = "servicebusnamespace"
	ServiceBusQueueNameKey                 = "servicebusqueue"
	ServiceBusQueueIDKey                   = "servicebusqueueid"
	ServiceBusTopicNameKey                 = "servicebustopic"
	ServiceBusTopicIDKey                   = "servicebustopicid"
	ServiceBusNamespaceConnectionStringKey = "servicebusnamespaceconnectionstring"
	ServiceBusQueueConnectionStringKey     = "servicebusqueueconnectionstring"
	RootManageSharedAccessKey              = "RootManageSharedAccessKey"
)

func NewAzureServiceBusQueueHandler(arm armauth.ArmConfig) ResourceHandler {
	handler := &azureServiceBusQueueHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
	}
	return handler
}

type azureServiceBusBaseHandler struct {
	arm armauth.ArmConfig
}

type azureServiceBusQueueHandler struct {
	azureServiceBusBaseHandler
}

func (handler *azureServiceBusQueueHandler) Put(ctx context.Context, options *PutOptions) (map[string]string, error) {
	logger := radlogger.GetLogger(ctx)
	logger.Info(fmt.Sprintf("Inside Put for Kind: %s", options.Resource.ResourceKind))
	properties := mergeProperties(*options.Resource, options.ExistingOutputResource)

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

	logger.Info(fmt.Sprintf("Validated unmanaged resource IDs - Namespace: %s, Queue: %s", ServiceBusNamespaceIDKey, ServiceBusQueueIDKey))
	var namespace *servicebus.SBNamespace
	if properties[ServiceBusNamespaceIDKey] == "" {
		// If we don't have an ID already then we will need to create a new one.
		namespace, err = handler.LookupSharedManagedNamespaceFromResourceGroup(ctx, options.ApplicationName)
		if err != nil {
			return nil, err
		}

		if namespace == nil {
			logger.Info(fmt.Sprintf("Creating namespace: %s", options.ApplicationName))
			namespace, err = handler.CreateNamespace(ctx, options.ApplicationName)
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

	var queueID string
	if properties[ServiceBusQueueIDKey] == "" {
		queue, err := handler.CreateQueue(ctx, *namespace.Name, queueName)
		if err != nil {
			return nil, err
		}
		properties[ServiceBusQueueIDKey] = *queue.ID
		queueID = *queue.ID
	} else {
		// This is mostly called for the side-effect of verifying that the servicebus queue exists.
		queue, err := handler.getQueueByID(ctx, properties[ServiceBusQueueIDKey])
		if err != nil {
			return nil, err
		}
		queueID = *queue.ID
	}

	namespaceConnectionString, err := handler.GetConnectionString(ctx, *namespace.Name)
	if err != nil {
		return nil, err
	}
	properties[ServiceBusNamespaceConnectionStringKey] = *namespaceConnectionString

	queueConnectionString, err := handler.GetQueueConnectionString(ctx, *namespace.Name, queueName)
	if err != nil {
		return nil, err
	}
	properties[ServiceBusQueueConnectionStringKey] = *queueConnectionString

	// Update the output resource with the info from the deployed Azure resource
	options.Resource.Identity = resourcemodel.NewARMIdentity(queueID, clients.GetAPIVersionFromUserAgent(servicebus.UserAgent()))

	return properties, nil
}

func (handler *azureServiceBusQueueHandler) Delete(ctx context.Context, options DeleteOptions) error {
	properties := options.ExistingOutputResource.PersistedProperties

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
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus queue resource id: %w", err)
	}

	sbc := clients.NewServiceBusNamespacesClient(parsed.SubscriptionID, handler.arm.Auth)

	// Check if a service bus namespace exists in the resource group for this application
	namespace, err := sbc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus namespace: %w", err)
	}

	return &namespace, nil
}

func (handler *azureServiceBusBaseHandler) LookupSharedManagedNamespaceFromResourceGroup(ctx context.Context, application string) (*servicebus.SBNamespace, error) {
	sbc := clients.NewServiceBusNamespacesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
	sbc := clients.NewServiceBusNamespacesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	location, err := clients.GetResourceGroupLocation(ctx, handler.arm)
	if err != nil {
		return nil, err
	}

	// Generate a random namespace name
	namespaceName := GenerateRandomName("radius-ns")

	future, err := sbc.CreateOrUpdate(ctx, handler.arm.ResourceGroup, namespaceName, servicebus.SBNamespace{
		Sku: &servicebus.SBSku{
			Name:     servicebus.Standard,
			Tier:     servicebus.SkuTierStandard,
			Capacity: to.Int32Ptr(1),
		},
		Location: location,

		// NOTE: this is a special case, we currently share servicebus resources per-application
		// they are not directly associated with a radius resource. See: #176
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
	tc := clients.NewTopicsClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus resource id: %w", err)
	}

	tc := clients.NewTopicsClient(handler.arm.SubscriptionID, handler.arm.Auth)

	topic, err := tc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus queue: %w", err)
	}

	return &topic, nil
}

func (handler *azureServiceBusBaseHandler) CreateQueue(ctx context.Context, namespaceName string, queueName string) (*servicebus.SBQueue, error) {
	qc := clients.NewQueuesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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

func (handler *azureServiceBusBaseHandler) getQueueByID(ctx context.Context, id string) (*servicebus.SBQueue, error) {
	parsed, err := azresources.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("failed to parse servicebus resource id: %w", err)
	}

	qc := clients.NewQueuesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	queue, err := qc.Get(ctx, parsed.ResourceGroup, parsed.Types[0].Name, parsed.Types[1].Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get servicebus queue: %w", err)
	}

	return &queue, nil
}

func (handler *azureServiceBusBaseHandler) GetConnectionString(ctx context.Context, namespaceName string) (*string, error) {
	sbc := clients.NewServiceBusNamespacesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	accessKeys, err := sbc.ListKeys(ctx, handler.arm.ResourceGroup, namespaceName, RootManageSharedAccessKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve connection strings")
	}

	return accessKeys.PrimaryConnectionString, nil
}

func (handler *azureServiceBusBaseHandler) GetQueueConnectionString(ctx context.Context, namespaceName string, queueName string) (*string, error) {
	sbc := clients.NewQueuesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	// Full access
	accessRights := []servicebus.AccessRights{"Listen", "Manage", "Send"}
	parameters := servicebus.SBAuthorizationRule{
		SBAuthorizationRuleProperties: &servicebus.SBAuthorizationRuleProperties{
			Rights: &accessRights,
		},
	}

	_, err := sbc.CreateOrUpdateAuthorizationRule(ctx, handler.arm.ResourceGroup, namespaceName, queueName, RootManageSharedAccessKey, parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to create queue authorization rule: %w", err)
	}

	accessKeys, err := sbc.ListKeys(ctx, handler.arm.ResourceGroup, namespaceName, queueName, RootManageSharedAccessKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve queue connection strings: %w", err)
	}

	if accessKeys.PrimaryConnectionString == nil {
		return nil, fmt.Errorf("failed to retrieve queue connection strings")
	}

	return accessKeys.PrimaryConnectionString, nil
}

func (handler *azureServiceBusBaseHandler) DeleteNamespace(ctx context.Context, namespaceName string) error {
	// The last queue in the service bus namespace was deleted. Now delete the namespace as well
	sbc := clients.NewServiceBusNamespacesClient(handler.arm.SubscriptionID, handler.arm.Auth)

	future, err := sbc.Delete(ctx, handler.arm.ResourceGroup, namespaceName)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "servicebus namespace", err)
	}

	err = future.WaitForCompletionRef(ctx, sbc.Client)
	if clients.IsLongRunning404(err, future.FutureAPI) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to delete %s: %w", "servicebus namespace", err)
	}

	return nil
}

// Returns true if the namespace can be deleted
func (handler *azureServiceBusBaseHandler) DeleteTopic(ctx context.Context, namespaceName string, topicName string) (bool, error) {
	tc := clients.NewTopicsClient(handler.arm.SubscriptionID, handler.arm.Auth)

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
	if tItr.NotDone() && *tItr.Value().Name != topicName {
		// There are other topics in the same service bus namespace. Do not remove the namespace as a part of this delete deployment
		return false, nil
	}

	// Namespace is empty, it can be deleted if it is unused
	return true, nil
}

// Returns true if the namespace can be deleted
func (handler *azureServiceBusBaseHandler) DeleteQueue(ctx context.Context, namespaceName, queueName string) (bool, error) {
	qc := clients.NewQueuesClient(handler.arm.SubscriptionID, handler.arm.Auth)

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

func NewAzureServiceBusQueueHealthHandler(arm armauth.ArmConfig) HealthHandler {
	handler := &azureServiceBusQueueHealthHandler{
		azureServiceBusBaseHandler: azureServiceBusBaseHandler{arm: arm},
	}
	return handler
}

type azureServiceBusQueueHealthHandler struct {
	azureServiceBusBaseHandler
}

func (handler *azureServiceBusQueueHealthHandler) GetHealthOptions(ctx context.Context) healthcontract.HealthCheckOptions {
	return healthcontract.HealthCheckOptions{}
}
