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
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/azure/clients"
	"github.com/project-radius/radius/pkg/healthcontract"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/resourcemodel"
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

	// This assertion is important so we don't start creating/modifying an resource
	err := ValidateResourceIDsForResource(properties, ServiceBusNamespaceIDKey, ServiceBusQueueIDKey)
	if err != nil {
		return nil, err
	}

	logger.Info(fmt.Sprintf("Validated resource IDs - Namespace: %s, Queue: %s", ServiceBusNamespaceIDKey, ServiceBusQueueIDKey))
	var namespace *servicebus.SBNamespace

	// This is mostly called for the side-effect of verifying that the servicebus namespace exists.
	namespace, err = handler.GetNamespaceByID(ctx, properties[ServiceBusNamespaceIDKey])
	if err != nil {
		return nil, err
	}

	var queueID string

	// This is mostly called for the side-effect of verifying that the servicebus queue exists.
	queue, err := handler.getQueueByID(ctx, properties[ServiceBusQueueIDKey])
	if err != nil {
		return nil, err
	}
	queueID = *queue.ID

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
