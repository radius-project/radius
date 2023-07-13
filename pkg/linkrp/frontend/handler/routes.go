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

	"github.com/go-chi/chi/v5"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/linkrp"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	dapr_dm "github.com/project-radius/radius/pkg/daprrp/datamodel"
	dapr_conv "github.com/project-radius/radius/pkg/daprrp/datamodel/converter"
	ds_dm "github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	ds_conv "github.com/project-radius/radius/pkg/datastoresrp/datamodel/converter"
	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	link_frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
	msg_dm "github.com/project-radius/radius/pkg/messagingrp/datamodel"
	msg_conv "github.com/project-radius/radius/pkg/messagingrp/datamodel/converter"
	msg_ctrl "github.com/project-radius/radius/pkg/messagingrp/frontend/controller/rabbitmqqueues"
)

const (
	LinkProviderNamespace       = "Applications.Link"
	DaprProviderNamespace       = "Applications.Dapr"
	DatastoresProviderNamespace = "Applications.Datastores"
	MessagingProviderNamespace  = "Applications.Messaging"
	resourceGroupPath           = "/resourcegroups/{resourceGroupName}"
)

// # Function Explanation
//
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

	err := AddLinkRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}

	err = AddMessagingRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}

	/* The following routes will be configured in upcoming PRs
	err = AddDatastoresRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}
	err = AddDaprRoutes(ctx, router, rootScopePath, prefixes, isARM, ctrlOpts)
	if err != nil {
		return err
	}
	*/

	return nil
}

// # Function Explanation
//
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
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
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
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
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
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
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
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[msg_dm.RabbitMQQueue]{
							rp_frontend.PrepareRadiusResource[*msg_dm.RabbitMQQueue],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:      msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:     msg_conv.RabbitMQQueueDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      rmqResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.N_RabbitMQQueuesResourceType,
			Method:            msg_ctrl.OperationListSecret,
			ControllerFactory: msg_ctrl.NewListSecretsRabbitMQQueue,
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// # Function Explanation
//
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

	pubsubPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/daprpubsubbrokers", validator)
	pubsubResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprpubsubbrokers", validator)
	pubsubResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprpubsubbrokers/{daprPubSubBrokerName}", validator)

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: pubsubPlaneRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
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
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
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
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
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
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprPubSubBroker],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprPubSubBroker],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:      dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter:     dapr_conv.PubSubBrokerDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
	}

	secretStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/daprsecretstores", validator)
	secretStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprsecretstores", validator)
	secretStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprsecretstores/{daprSecretStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: secretStorePlaneRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
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
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
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
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
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
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprSecretStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprSecretStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:      dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter:     dapr_conv.SecretStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprSecretStoreTimeout,
					},
				)
			},
		},
	}...)

	stateStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.dapr/daprstatestores", validator)
	stateStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprstatestores", validator)
	stateStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.dapr/daprstatestores/{daprStateStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: stateStorePlaneRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
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
			ResourceType: linkrp.N_DaprStateStoresResourceType,
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
			ResourceType: linkrp.N_DaprStateStoresResourceType,
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
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprStateStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[dapr_dm.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*dapr_dm.DaprStateStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:      dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter:     dapr_conv.StateStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprStateStoreTimeout,
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

// # Function Explanation
//
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
			ResourceType: linkrp.N_MongoDatabasesResourceType,
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
			ResourceType: linkrp.N_MongoDatabasesResourceType,
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
			ResourceType: linkrp.N_MongoDatabasesResourceType,
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
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.MongoDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:      ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:     ds_conv.MongoDatabaseDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      mongoResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.MongoDatabasesResourceType,
			Method:            mongo_ctrl.OperationListSecret,
			ControllerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
	}

	redisPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.datastores/rediscaches", validator)
	redisResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/rediscaches", validator)
	redisResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/rediscaches/{redisCacheName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: redisPlaneRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
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
			ResourceType: linkrp.N_RedisCachesResourceType,
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
			ResourceType: linkrp.N_RedisCachesResourceType,
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
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.RedisCache]{
							rp_frontend.PrepareRadiusResource[*ds_dm.RedisCache],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.RedisCache]{
							rp_frontend.PrepareRadiusResource[*ds_dm.RedisCache],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:      ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter:     ds_conv.RedisCacheDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      redisResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.RedisCachesResourceType,
			Method:            redis_ctrl.OperationListSecret,
			ControllerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
	}...)

	sqlPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.datastores/sqldatabases", validator)
	sqlResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/sqldatabases", validator)
	sqlResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.datastores/sqldatabases/{sqlDatabaseName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: sqlPlaneRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
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
			ResourceType: linkrp.N_SqlDatabasesResourceType,
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
			ResourceType: linkrp.N_SqlDatabasesResourceType,
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
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.SqlDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[ds_dm.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*ds_dm.SqlDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:      ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:     ds_conv.SqlDatabaseDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteSqlDatabaseTimeout,
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

// # Function Explanation
//
// AddLinkRoutes sets up routes and registers handlers for various operations (GET, PUT, PATCH, DELETE) on different
// resources (MongoDatabases, DaprPubSubBrokers, DaprSecretStores, DaprStateStores, Extenders, RedisCaches,
// RabbitMQMessageQueues and SQLDatabases). It also sets up the necessary options for each handler.
func AddLinkRoutes(ctx context.Context, r chi.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {
	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, r, rootScopePath, isARM, LinkProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, LinkProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
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

	mongoPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/mongodatabases", validator)
	mongoResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/mongodatabases", validator)
	mongoResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/mongodatabases/{mongoDatabaseName}", validator)

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: mongoPlaneRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:   converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:  converter.MongoDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: mongoResourceGroupRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:      converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:     converter.MongoDatabaseDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteMongoDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      mongoResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.MongoDatabasesResourceType,
			Method:            mongo_ctrl.OperationListSecret,
			ControllerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
	}

	pubsubPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/daprpubsubbrokers", validator)
	pubsubResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprpubsubbrokers", validator)
	pubsubResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprpubsubbrokers/{daprPubSubBrokerName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: pubsubPlaneRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:   converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter:  converter.DaprPubSubBrokerDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: pubsubResourceGroupRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprPubSubBroker],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprPubSubBroker]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprPubSubBroker],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
		{
			ParentRouter: pubsubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:      converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter:     converter.DaprPubSubBrokerDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
	}...)

	secretStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/daprsecretstores", validator)
	secretStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprsecretstores", validator)
	secretStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprsecretstores/{daprSecretStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: secretStorePlaneRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:   converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter:  converter.DaprSecretStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceGroupRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprSecretStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprSecretStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprSecretStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprSecretStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: secretStoreResourceRouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:      converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter:     converter.DaprSecretStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprSecretStoreTimeout,
					},
				)
			},
		},
	}...)

	stateStorePlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/daprstatestores", validator)
	stateStoreResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprstatestores", validator)
	stateStoreResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/daprstatestores/{daprStateStoreName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: stateStorePlaneRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:   converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter:  converter.DaprStateStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceGroupRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprStateStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.DaprStateStore]{
							rp_frontend.PrepareRadiusResource[*datamodel.DaprStateStore],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprStateStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: stateStoreResourceRouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:      converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter:     converter.DaprStateStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprStateStoreTimeout,
					},
				)
			},
		},
	}...)

	redisPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/rediscaches", validator)
	redisResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/rediscaches", validator)
	redisResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/rediscaches/{redisCacheName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: redisPlaneRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:   converter.RedisCacheDataModelFromVersioned,
						ResponseConverter:  converter.RedisCacheDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: redisResourceGroupRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RedisCache]{
							rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RedisCache]{
							rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:      converter.RedisCacheDataModelFromVersioned,
						ResponseConverter:     converter.RedisCacheDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteRedisCacheTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      redisResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.RedisCachesResourceType,
			Method:            redis_ctrl.OperationListSecret,
			ControllerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
	}...)

	rmqPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/rabbitmqmessagequeues", validator)
	rmqResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/rabbitmqmessagequeues", validator)
	rmqResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/rabbitmqmessagequeues/{rabbitMQMessageQueueName}", validator)

	// Messaging handlers:
	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: rmqPlaneRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:   converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter:  converter.RabbitMQMessageQueueDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: rmqResourceGroupRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RabbitMQMessageQueue]{
							rp_frontend.PrepareRadiusResource[*datamodel.RabbitMQMessageQueue],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RabbitMQMessageQueue]{
							rp_frontend.PrepareRadiusResource[*datamodel.RabbitMQMessageQueue],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter: rmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:      converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter:     converter.RabbitMQMessageQueueDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteRabbitMQTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      rmqResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.RabbitMQMessageQueuesResourceType,
			Method:            rabbitmq_ctrl.OperationListSecret,
			ControllerFactory: rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue,
		},
	}...)

	sqlPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/sqldatabases", validator)
	sqlResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/sqldatabases", validator)
	sqlResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/sqldatabases/{sqlDatabaseName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: sqlPlaneRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:   converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:  converter.SqlDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: sqlResourceGroupRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.SqlDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.SqlDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.SqlDatabase],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateSqlDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:      converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:     converter.SqlDatabaseDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteSqlDatabaseTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      sqlResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.SqlDatabasesResourceType,
			Method:            sql_ctrl.OperationListSecret,
			ControllerFactory: sql_ctrl.NewListSecretsSqlDatabase,
		},
	}...)

	extPlaneRouter := server.NewSubrouter(r, rootScopePath+"/providers/applications.link/extenders", validator)
	extResourceGroupRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/extenders", validator)
	extResourceRouter := server.NewSubrouter(r, rootScopePath+resourceGroupPath+"/providers/applications.link/extenders/{extenderName}", validator)

	handlerOptions = append(handlerOptions, []server.HandlerOptions{
		{
			ParentRouter: extPlaneRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:   converter.ExtenderDataModelFromVersioned,
						ResponseConverter:  converter.ExtenderDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: extResourceGroupRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationList,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: extResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationGet,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: extResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationPut,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Extender]{
							rp_frontend.PrepareRadiusResource[*datamodel.Extender],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateExtenderTimeout,
					},
				)
			},
		},
		{
			ParentRouter: extResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationPatch,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.Extender]{
							rp_frontend.PrepareRadiusResource[*datamodel.Extender],
						},
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateExtenderTimeout,
					},
				)
			},
		},
		{
			ParentRouter: extResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationDelete,
			ControllerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:      converter.ExtenderDataModelFromVersioned,
						ResponseConverter:     converter.ExtenderDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteExtenderTimeout,
					},
				)
			},
		},
		{
			ParentRouter:      extResourceRouter,
			Path:              "/listsecrets",
			ResourceType:      linkrp.ExtendersResourceType,
			Method:            extender_ctrl.OperationListSecret,
			ControllerFactory: extender_ctrl.NewListSecretsExtender,
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
