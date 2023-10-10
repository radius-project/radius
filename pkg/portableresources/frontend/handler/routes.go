/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0


Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package handler

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	rp_frontend "github.com/radius-project/radius/pkg/rp/frontend"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"

	dapr_dm "github.com/radius-project/radius/pkg/daprrp/datamodel"
	dapr_conv "github.com/radius-project/radius/pkg/daprrp/datamodel/converter"
	dapr_ctrl "github.com/radius-project/radius/pkg/daprrp/frontend/controller"
	ds_dm "github.com/radius-project/radius/pkg/datastoresrp/datamodel"
	ds_conv "github.com/radius-project/radius/pkg/datastoresrp/datamodel/converter"
	ds_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller"
	mongo_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/mongodatabases"
	redis_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/radius-project/radius/pkg/datastoresrp/frontend/controller/sqldatabases"
	msg_dm "github.com/radius-project/radius/pkg/messagingrp/datamodel"
	msg_conv "github.com/radius-project/radius/pkg/messagingrp/datamodel/converter"
	msg_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller"
	rmq_ctrl "github.com/radius-project/radius/pkg/messagingrp/frontend/controller/rabbitmqqueues"
)

const (
	// resourceGroupPath is the path for resource groups.
	resourceGroupPath = "/resourcegroups/{resourceGroupName}"

	// PortableResourcesNamespace is the name representing group of portable resource providers.
	PortableResourcesNamespace = "Applications.Datastores"

	// DaprProviderNamespace is the namespace for Dapr provider.
	DaprProviderNamespace = "Applications.Dapr"

	// DatastoresProviderNamespace is the namespace for Datastores provider.
	DatastoresProviderNamespace = "Applications.Datastores"

	// MessagingProviderNamespace is the namespace for Messaging provider.
	MessagingProviderNamespace = "Applications.Messaging"

	// AsyncOperationRetryAfter is the polling interval for async create/update or delete resource operations.
	AsyncOperationRetryAfter = time.Duration(5) * time.Second
)

// AddRoutes configures routes and handlers for Datastores, Messaging, Dapr Resource Providers.
func AddRoutes(ctx context.Context, router chi.Router, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	rootScopePath := ctrlOpts.PathBase
	rootScopePath += getRootScopePath(isARM)

	// URLs may use either the subscription/plane scope or resource group scope.
	// These paths are order sensitive and the longer path MUST be registered first.
	prefixes := []string{
		rootScopePath + resourceGroupPath,
		rootScopePath,
	}

	err := AddMessagingRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}

	err = AddDaprRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}

	err = AddDatastoresRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}

	return nil
}

// AddMessagingRoutes configures the default ARM handlers and registers handlers for the RabbitMQQueue resource type for
// the List, Get, Put, Patch and Delete operations.
func AddMessagingRoutes(ctx context.Context, r chi.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, r, rootScopePath, isARM, MessagingProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, MessagingProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	validator := validator.APIValidator(validator.Options{
		SpecLoader:         specLoader,
		ResourceTypeGetter: validator.RadiusResourceTypeGetter,
	})

	// Register resource routers.
	//
	// Note: We have to follow the below rules to enable API validators:
	// 1. For collection scope routers (xxxPlaneRouter and xxxResourceGroupRouter), register validator at HandlerOptions.Middlewares.
	// 2. For resource scopes (xxxResourceRouter), register validator at Subrouter.

	// rabbitmqqueues router handlers:
	rmqPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.messaging/rabbitmqqueues", validator)
	rmqResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.messaging/rabbitmqqueues", validator)
	rmqResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.messaging/rabbitmqqueues/{rabbitMQQueueName}", validator)

	// Messaging handlers:
	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: rmqPlaneRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:   msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:  msg_conv.RabbitMQQueueDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: rmqResourceGroupRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout:    msg_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout:    msg_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: msg_ctrl.RabbitMQQueuesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:         msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:        msg_conv.RabbitMQQueueDataModelToVersioned,
						AsyncOperationTimeout:    msg_ctrl.AsyncDeleteRabbitMQTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter:      rmqResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      msg_ctrl.RabbitMQQueuesResourceType,
			Method:            msg_ctrl.OperationListSecret,
			ControllerFactory: rmq_ctrl.NewListSecretsRabbitMQQueue,
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// AddDaprRoutes configures the default ARM handlers and adds handlers for Dapr resources such as Dapr PubSubBroker,
// SecretStore and StateStore. It registers handlers for various operations on these resources.
func AddDaprRoutes(ctx context.Context, r chi.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {

	// Dapr - Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, r, rootScopePath, isARM, DaprProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, DaprProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	validator := validator.APIValidator(validator.Options{
		SpecLoader:         specLoader,
		ResourceTypeGetter: validator.RadiusResourceTypeGetter,
	})

	// Register resource routers.
	//
	// Note: We have to follow the below rules to enable API validators:
	// 1. For collection scope routers (xxxPlaneRouter and xxxResourceGroupRouter), register validator at HandlerOptions.Middlewares.
	// 2. For resource scopes (xxxResourceRouter), register validator at Subrouter.

	pubsubPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/pubsubbrokers", validator)
	pubsubResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/pubsubbrokers", validator)
	pubsubResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/pubsubbrokers/{pubSubBrokerName}", validator)

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: pubsubPlaneRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:   dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter:  dapr_conv.PubSubBrokerDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: pubsubResourceGroupRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprPubSubBroker],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprPubSubBroker],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprPubSubBroker],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprPubSubBroker],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: dapr_ctrl.DaprPubSubBrokersResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:         dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter:        dapr_conv.PubSubBrokerDataModelToVersioned,
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
	}

	secretStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/secretstores", validator)
	secretStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/secretstores", validator)
	secretStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/secretstores/{secretStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: secretStorePlaneRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:   dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter:  dapr_conv.SecretStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceGroupRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprSecretStore],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprSecretStore],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprSecretStore],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprSecretStore],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprSecretStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:         dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter:        dapr_conv.SecretStoreDataModelToVersioned,
						AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprSecretStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
	}...)

	stateStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/statestores", validator)
	stateStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/statestores", validator)
	stateStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/statestores/{stateStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: stateStorePlaneRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:   dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter:  dapr_conv.StateStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceGroupRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprStateStore],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprStateStore],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprStateStore],
							rp_frontend.PrepareDaprResource[*dapr_dm.DaprStateStore],
						},
						AsyncOperationTimeout:    dapr_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: dapr_ctrl.DaprStateStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:         dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter:        dapr_conv.StateStoreDataModelToVersioned,
						AsyncOperationTimeout:    dapr_ctrl.AsyncDeleteDaprStateStoreTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
	}...)

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// AddDatastoresRoutes configures the routes and handlers for  Datastores Resource Provider. It registers handlers for List, Get, Put,
// Patch, and Delete operations for MongoDatabase, RedisCache, and SqlDatabase resources.
func AddDatastoresRoutes(ctx context.Context, r chi.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	// Datastores - Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, r, rootScopePath, isARM, DatastoresProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, DatastoresProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	validator := validator.APIValidator(validator.Options{
		SpecLoader:         specLoader,
		ResourceTypeGetter: validator.RadiusResourceTypeGetter,
	})

	// Register resource routers.
	//
	// Note: We have to follow the below rules to enable API validators:
	// 1. For collection scope routers (xxxPlaneRouter and xxxResourceGroupRouter), register validator at HandlerOptions.Middlewares.
	// 2. For resource scopes (xxxResourceRouter), register validator at Subrouter.

	mongoPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.datastores/mongodatabases", validator)
	mongoResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/mongodatabases", validator)
	mongoResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/mongodatabases/{mongoDatabaseName}", validator)

	// Datastores handlers:
	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: mongoPlaneRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:   ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:  ds_conv.MongoDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: mongoResourceGroupRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.MongoDatabase],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.MongoDatabase],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: ds_ctrl.MongoDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:         ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:        ds_conv.MongoDatabaseDataModelToVersioned,
						AsyncOperationTimeout:    ds_ctrl.AsyncDeleteMongoDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter:      mongoResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      ds_ctrl.MongoDatabasesResourceType,
			Method:            ds_ctrl.OperationListSecret,
			ControllerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
	}

	redisPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.datastores/rediscaches", validator)
	redisResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/rediscaches", validator)
	redisResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/rediscaches/{redisCacheName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: redisPlaneRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:   ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter:  ds_conv.RedisCacheDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: redisResourceGroupRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.RedisCache]{
							rp_frontend.PrepareRadiusResource[*ds_dm.RedisCache],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.RedisCache]{
							rp_frontend.PrepareRadiusResource[*ds_dm.RedisCache],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: ds_ctrl.RedisCachesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:         ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter:        ds_conv.RedisCacheDataModelToVersioned,
						AsyncOperationTimeout:    ds_ctrl.AsyncDeleteRedisCacheTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter:      redisResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      ds_ctrl.RedisCachesResourceType,
			Method:            ds_ctrl.OperationListSecret,
			ControllerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
	}...)

	sqlPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.datastores/sqldatabases", validator)
	sqlResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/sqldatabases", validator)
	sqlResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/sqldatabases/{sqlDatabaseName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: sqlPlaneRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:   ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:  ds_conv.SqlDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: sqlResourceGroupRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.SqlDatabase],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.SqlDatabase],
						},
						AsyncOperationTimeout:    ds_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: ds_ctrl.SqlDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:         ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:        ds_conv.SqlDatabaseDataModelToVersioned,
						AsyncOperationTimeout:    ds_ctrl.AsyncDeleteSqlDatabaseTimeout,
						AsyncOperationRetryAfter: AsyncOperationRetryAfter,
					},
				)
			},
		},
		{
			ParentRouter:      sqlResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      ds_ctrl.SqlDatabasesResourceType,
			Method:            ds_ctrl.OperationListSecret,
			ControllerFactory: sql_ctrl.NewListSecretsSqlDatabase,
		},
	}...)

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

func getRootScopePath(isARM bool) string {
	if isARM {
		return "/subscriptions/{subscriptionID}"
	}
	return "/planes/radius/{planeName}"
}
