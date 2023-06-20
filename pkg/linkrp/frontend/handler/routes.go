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

	"github.com/gorilla/mux"
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

// AddRoutes configures routes and handlers for resourceproviders Datastoresrp, Messagingrp, Daprrp.
func AddRoutes(ctx context.Context, router *mux.Router, isARM bool, ctrlOpts frontend_ctrl.Options) error {
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

// AddMessagingRoutes configures routes and handlers for Messaging Resource Provider.
func AddMessagingRoutes(ctx context.Context, router *mux.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, rootScopePath, isARM, MessagingProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	msg_specLoader, err := validator.LoadSpec(ctx, MessagingProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	planeScopeRouter := router.PathPrefix(rootScopePath).Subrouter()
	planeScopeRouter.Use(validator.APIValidator(msg_specLoader))

	resourceGroupScopeRouter := router.PathPrefix(rootScopePath + resourceGroupPath).Subrouter()
	resourceGroupScopeRouter.Use(validator.APIValidator(msg_specLoader))

	rabbitmqQueuePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.messaging/rabbitmqqueues").Subrouter()
	rabbitmqQueueResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.messaging/rabbitmqqueues").Subrouter()
	rabbitmqQueueResourceRouter := rabbitmqQueueResourceGroupRouter.PathPrefix("/{rabbitMQQueueName}").Subrouter()

	// Messaging handlers:
	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: rabbitmqQueuePlaneRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:   msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter:  msg_conv.RabbitMQQueueDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: rabbitmqQueueResourceGroupRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rabbitmqQueueResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[msg_dm.RabbitMQQueue]{
						RequestConverter:  msg_conv.RabbitMQQueueDataModelFromVersioned,
						ResponseConverter: msg_conv.RabbitMQQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rabbitmqQueueResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: rabbitmqQueueResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: rabbitmqQueueResourceRouter,
			ResourceType: linkrp.N_RabbitMQQueuesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter:   rabbitmqQueueResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   linkrp.N_RabbitMQQueuesResourceType,
			Method:         msg_ctrl.OperationListSecret,
			HandlerFactory: msg_ctrl.NewListSecretsRabbitMQQueue,
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// AddDaprRoutes configures routes and handlers for Dapr Resource Provider.
func AddDaprRoutes(ctx context.Context, router *mux.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {

	// Dapr - Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, rootScopePath, isARM, DaprProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	dapr_specLoader, err := validator.LoadSpec(ctx, DaprProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	planeScopeRouter := router.PathPrefix(rootScopePath).Subrouter()
	planeScopeRouter.Use(validator.APIValidator(dapr_specLoader))

	resourceGroupScopeRouter := router.PathPrefix(rootScopePath + resourceGroupPath).Subrouter()
	resourceGroupScopeRouter.Use(validator.APIValidator(dapr_specLoader))

	daprPubSubBrokerPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.dapr/daprpubsubbrokers").Subrouter()
	daprPubSubBrokerResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.dapr/daprpubsubbrokers").Subrouter()
	daprPubSubBrokerResourceRouter := daprPubSubBrokerResourceGroupRouter.PathPrefix("/{daprPubSubBrokerName}").Subrouter()

	daprSecretStorePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.dapr/daprsecretstores").Subrouter()
	daprSecretStoreResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.dapr/daprsecretstores").Subrouter()
	daprSecretStoreResourceRouter := daprSecretStoreResourceGroupRouter.PathPrefix("/{daprSecretStoreName}").Subrouter()

	daprStateStorePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.dapr/daprstatestores").Subrouter()
	daprStateStoreResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.dapr/daprstatestores").Subrouter()
	daprStateStoreResourceRouter := daprStateStoreResourceGroupRouter.PathPrefix("/{daprStateStoreName}").Subrouter()

	// Dapr handlers:
	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: daprPubSubBrokerPlaneRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:   dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter:  dapr_conv.PubSubBrokerDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: daprPubSubBrokerResourceGroupRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprPubSubBrokerResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:  dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter: dapr_conv.PubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprPubSubBrokerResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprPubSubBrokerResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprPubSubBrokerResourceRouter,
			ResourceType: linkrp.N_DaprPubSubBrokersResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprPubSubBroker]{
						RequestConverter:      dapr_conv.PubSubBrokerDataModelFromVersioned,
						ResponseConverter:     dapr_conv.PubSubBrokerDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncCreateOrUpdateDaprPubSubBrokerTimeout,
					},
				)
			},
		},
		{
			ParentRouter: daprSecretStorePlaneRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:   dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter:  dapr_conv.SecretStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: daprSecretStoreResourceGroupRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:  dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.SecretStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: linkrp.N_DaprSecretStoresResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprSecretStore]{
						RequestConverter:      dapr_conv.SecretStoreDataModelFromVersioned,
						ResponseConverter:     dapr_conv.SecretStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprSecretStoreTimeout,
					},
				)
			},
		},
		{
			ParentRouter: daprStateStorePlaneRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:   dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter:  dapr_conv.StateStoreDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: daprStateStoreResourceGroupRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:  dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter: dapr_conv.StateStoreDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: linkrp.N_DaprStateStoresResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[dapr_dm.DaprStateStore]{
						RequestConverter:      dapr_conv.StateStoreDataModelFromVersioned,
						ResponseConverter:     dapr_conv.StateStoreDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteDaprStateStoreTimeout,
					},
				)
			},
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// AddDatastoresRoutes configures the routes and handlers for Datastores Resource Provider.
func AddDatastoresRoutes(ctx context.Context, router *mux.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {

	// Datastores - Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, rootScopePath, isARM, DatastoresProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	ds_specLoader, err := validator.LoadSpec(ctx, DatastoresProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	planeScopeRouter := router.PathPrefix(rootScopePath).Subrouter()
	planeScopeRouter.Use(validator.APIValidator(ds_specLoader))

	resourceGroupScopeRouter := router.PathPrefix(rootScopePath + resourceGroupPath).Subrouter()
	resourceGroupScopeRouter.Use(validator.APIValidator(ds_specLoader))

	mongoDatabasePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.datastores/mongodatabases").Subrouter()
	mongoDatabaseResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.datastores/mongodatabases").Subrouter()
	mongoDatabaseResourceRouter := mongoDatabaseResourceGroupRouter.PathPrefix("/{mongoDatabaseName}").Subrouter()

	redisCachePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.datastores/rediscaches").Subrouter()
	redisCacheResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.datastores/rediscaches").Subrouter()
	redisCacheResourceRouter := redisCacheResourceGroupRouter.PathPrefix("/{redisCacheName}").Subrouter()

	sqlDatabasePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.datastores/sqldatabases").Subrouter()
	sqlDatabaseResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.datastores/sqldatabases").Subrouter()
	sqlDatabaseResourceRouter := sqlDatabaseResourceGroupRouter.PathPrefix("/{sqlDatabaseName}").Subrouter()

	// Datastores handlers:
	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: mongoDatabasePlaneRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:   ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter:  ds_conv.MongoDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: mongoDatabaseResourceGroupRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoDatabaseResourceRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.MongoDatabase]{
						RequestConverter:  ds_conv.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.MongoDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: mongoDatabaseResourceRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: mongoDatabaseResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: mongoDatabaseResourceRouter,
			ResourceType: linkrp.N_MongoDatabasesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter:   mongoDatabaseResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   linkrp.MongoDatabasesResourceType,
			Method:         mongo_ctrl.OperationListSecret,
			HandlerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
		{
			ParentRouter: redisCachePlaneRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:   ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter:  ds_conv.RedisCacheDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: redisCacheResourceGroupRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisCacheResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.RedisCache]{
						RequestConverter:  ds_conv.RedisCacheDataModelFromVersioned,
						ResponseConverter: ds_conv.RedisCacheDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: redisCacheResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: redisCacheResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: redisCacheResourceRouter,
			ResourceType: linkrp.N_RedisCachesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter:   redisCacheResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   linkrp.RedisCachesResourceType,
			Method:         redis_ctrl.OperationListSecret,
			HandlerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
		{
			ParentRouter: sqlDatabasePlaneRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:   ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:  ds_conv.SqlDatabaseDataModelToVersioned,
						ListRecursiveQuery: true,
					})
			},
		},
		{
			ParentRouter: sqlDatabaseResourceGroupRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlDatabaseResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:  ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: ds_conv.SqlDatabaseDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: sqlDatabaseResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: sqlDatabaseResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: sqlDatabaseResourceRouter,
			ResourceType: linkrp.N_SqlDatabasesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[ds_dm.SqlDatabase]{
						RequestConverter:      ds_conv.SqlDatabaseDataModelFromVersioned,
						ResponseConverter:     ds_conv.SqlDatabaseDataModelToVersioned,
						AsyncOperationTimeout: link_frontend_ctrl.AsyncDeleteSqlDatabaseTimeout,
					},
				)
			},
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}

// AddLinkRoutes configures routes and handlers for the Link Resource Provider.
func AddLinkRoutes(ctx context.Context, router *mux.Router, rootScopePath string, prefixes []string, isARM bool, ctrlOpts frontend_ctrl.Options) error {

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, rootScopePath, isARM, LinkProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, LinkProviderNamespace, swagger.SpecFiles, prefixes, "rootScope")
	if err != nil {
		return err
	}

	// Used to register routes like:
	//
	// /planes/radius/{planeName}/providers/applications.link/mongodatabases
	planeScopeRouter := router.PathPrefix(rootScopePath).Subrouter()
	planeScopeRouter.Use(validator.APIValidator(specLoader))

	// Used to register routes like:
	//
	// /planes/radius/{planeName}/resourcegroups/{resourceGroupName}/providers/applications.link/mongodatabases
	resourceGroupScopeRouter := router.PathPrefix(rootScopePath + resourceGroupPath).Subrouter()
	resourceGroupScopeRouter.Use(validator.APIValidator(specLoader))

	mongoDatabasePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/mongodatabases").Subrouter()
	mongoDatabaseResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/mongodatabases").Subrouter()
	mongoDatabaseResourceRouter := mongoDatabaseResourceGroupRouter.PathPrefix("/{mongoDatabaseName}").Subrouter()

	daprPubSubBrokerPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/daprpubsubbrokers").Subrouter()
	daprPubSubBrokerResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/daprpubsubbrokers").Subrouter()
	daprPubSubBrokerResourceRouter := daprPubSubBrokerResourceGroupRouter.PathPrefix("/{daprPubSubBrokerName}").Subrouter()

	daprSecretStorePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/daprsecretstores").Subrouter()
	daprSecretStoreResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/daprsecretstores").Subrouter()
	daprSecretStoreResourceRouter := daprSecretStoreResourceGroupRouter.PathPrefix("/{daprSecretStoreName}").Subrouter()

	daprStateStorePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/daprstatestores").Subrouter()
	daprStateStoreResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/daprstatestores").Subrouter()
	daprStateStoreResourceRouter := daprStateStoreResourceGroupRouter.PathPrefix("/{daprStateStoreName}").Subrouter()

	extenderPlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/extenders").Subrouter()
	extenderResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/extenders").Subrouter()
	extenderResourceRouter := extenderResourceGroupRouter.PathPrefix("/{extenderName}").Subrouter()

	redisCachePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/rediscaches").Subrouter()
	redisCacheResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/rediscaches").Subrouter()
	redisCacheResourceRouter := redisCacheResourceGroupRouter.PathPrefix("/{redisCacheName}").Subrouter()

	rabbitmqMessageQueuePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/rabbitmqmessagequeues").Subrouter()
	rabbitmqMessageQueueResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/rabbitmqmessagequeues").Subrouter()
	rabbitmqMessageQueueResourceRouter := rabbitmqMessageQueueResourceGroupRouter.PathPrefix("/{rabbitMQMessageQueueName}").Subrouter()

	sqlDatabasePlaneRouter := planeScopeRouter.PathPrefix("/providers/applications.link/sqldatabases").Subrouter()
	sqlDatabaseResourceGroupRouter := resourceGroupScopeRouter.PathPrefix("/providers/applications.link/sqldatabases").Subrouter()
	sqlDatabaseResourceRouter := sqlDatabaseResourceGroupRouter.PathPrefix("/{sqlDatabaseName}").Subrouter()

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: mongoDatabasePlaneRouter,
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
			ParentRouter: mongoDatabaseResourceGroupRouter,
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
			ParentRouter: mongoDatabaseResourceRouter,
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
			ParentRouter: mongoDatabaseResourceRouter,
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
			ParentRouter: mongoDatabaseResourceRouter,
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
			ParentRouter: mongoDatabaseResourceRouter,
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
			ParentRouter:      mongoDatabaseResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:      linkrp.MongoDatabasesResourceType,
			Method:            mongo_ctrl.OperationListSecret,
			ControllerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
		{
			ParentRouter: daprPubSubBrokerPlaneRouter,
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
			ParentRouter: daprPubSubBrokerResourceGroupRouter,
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
			ParentRouter: daprPubSubBrokerResourceRouter,
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
			ParentRouter: daprPubSubBrokerResourceRouter,
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
			ParentRouter: daprPubSubBrokerResourceRouter,
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
			ParentRouter: daprPubSubBrokerResourceRouter,
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
		{
			ParentRouter: daprSecretStorePlaneRouter,
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
			ParentRouter: daprSecretStoreResourceGroupRouter,
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
			ParentRouter: daprSecretStoreResourceRouter,
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
			ParentRouter: daprSecretStoreResourceRouter,
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
			ParentRouter: daprSecretStoreResourceRouter,
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
			ParentRouter: daprSecretStoreResourceRouter,
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
		{
			ParentRouter: daprStateStorePlaneRouter,
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
			ParentRouter: daprStateStoreResourceGroupRouter,
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
			ParentRouter: daprStateStoreResourceRouter,
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
			ParentRouter: daprStateStoreResourceRouter,
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
			ParentRouter: daprStateStoreResourceRouter,
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
			ParentRouter: daprStateStoreResourceRouter,
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
		{
			ParentRouter: redisCachePlaneRouter,
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
			ParentRouter: redisCacheResourceGroupRouter,
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
			ParentRouter: redisCacheResourceRouter,
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
			ParentRouter: redisCacheResourceRouter,
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
			ParentRouter: redisCacheResourceRouter,
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
			ParentRouter: redisCacheResourceRouter,
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
			ParentRouter:      redisCacheResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:      linkrp.RedisCachesResourceType,
			Method:            redis_ctrl.OperationListSecret,
			ControllerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
		{
			ParentRouter: rabbitmqMessageQueuePlaneRouter,
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
			ParentRouter: rabbitmqMessageQueueResourceGroupRouter,
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
			ParentRouter: rabbitmqMessageQueueResourceRouter,
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
			ParentRouter: rabbitmqMessageQueueResourceRouter,
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
			ParentRouter: rabbitmqMessageQueueResourceRouter,
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
			ParentRouter: rabbitmqMessageQueueResourceRouter,
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
			ParentRouter:      rabbitmqMessageQueueResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:      linkrp.RabbitMQMessageQueuesResourceType,
			Method:            rabbitmq_ctrl.OperationListSecret,
			ControllerFactory: rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue,
		},
		{
			ParentRouter: sqlDatabasePlaneRouter,
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
			ParentRouter: sqlDatabaseResourceGroupRouter,
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
			ParentRouter: sqlDatabaseResourceRouter,
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
			ParentRouter: sqlDatabaseResourceRouter,
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
			ParentRouter: sqlDatabaseResourceRouter,
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
			ParentRouter: sqlDatabaseResourceRouter,
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
			ParentRouter:      sqlDatabaseResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:      linkrp.SqlDatabasesResourceType,
			Method:            sql_ctrl.OperationListSecret,
			ControllerFactory: sql_ctrl.NewListSecretsSqlDatabase,
		},
		{
			ParentRouter: extenderPlaneRouter,
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
			ParentRouter: extenderResourceGroupRouter,
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
			ParentRouter: extenderResourceRouter,
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
			ParentRouter: extenderResourceRouter,
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
			ParentRouter: extenderResourceRouter,
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
			ParentRouter: extenderResourceRouter,
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
			ParentRouter:      extenderResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:      linkrp.ExtendersResourceType,
			Method:            extender_ctrl.OperationListSecret,
			ControllerFactory: extender_ctrl.NewListSecretsExtender,
		},
	}

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
