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

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	link_frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	daprHttpRoute_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprinvokehttproutes"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

const (
	ProviderNamespaceName = "Applications.Link"
)

func AddRoutes(ctx context.Context, router *mux.Router, pathBase string, isARM bool, ctrlOpts frontend_ctrl.Options, dp deployment.DeploymentProcessor) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	} else {
		pathBase += "/planes/radius/{planeName}"
	}
	resourceGroupPath := "/resourcegroups/{resourceGroupName}"

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, ProviderNamespaceName, swagger.SpecFiles, pathBase+resourceGroupPath, "rootScope")
	if err != nil {
		return err
	}

	rootScopeRouter := router.PathPrefix(pathBase + resourceGroupPath).Subrouter()
	rootScopeRouter.Use(validator.APIValidator(specLoader))

	mongoRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/mongodatabases").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.PathPrefix("/{mongoDatabaseName}").Subrouter()

	daprHttpRouteRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/daprinvokehttproutes").Subrouter()
	daprHttpRouteResourceRouter := daprHttpRouteRTSubrouter.PathPrefix("/{daprInvokeHttpRouteName}").Subrouter()

	daprPubSubRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/daprpubsubbrokers").Subrouter()
	daprPubSubResourceRouter := daprPubSubRTSubrouter.PathPrefix("/{daprPubSubBrokerName}").Subrouter()

	daprSecretStoreRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/daprsecretstores").Subrouter()
	daprSecretStoreResourceRouter := daprSecretStoreRTSubrouter.PathPrefix("/{daprSecretStoreName}").Subrouter()

	daprStateStoreRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/daprstatestores").Subrouter()
	daprStateStoreResourceRouter := daprStateStoreRTSubrouter.PathPrefix("/{daprStateStoreName}").Subrouter()

	extenderRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/extenders").Subrouter()
	extenderResourceRouter := extenderRTSubrouter.PathPrefix("/{extenderName}").Subrouter()

	redisRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/rediscaches").Subrouter()
	redisResourceRouter := redisRTSubrouter.PathPrefix("/{redisCacheName}").Subrouter()

	rabbitmqRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/rabbitmqmessagequeues").Subrouter()
	rabbitmqResourceRouter := rabbitmqRTSubrouter.PathPrefix("/{rabbitMQMessageQueueName}").Subrouter()

	sqlRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.link/sqldatabases").Subrouter()
	sqlResourceRouter := sqlRTSubrouter.PathPrefix("/{sqlDatabaseName}").Subrouter()

	handlerOptions := []server.HandlerOptions{
		{
			ParentRouter: mongoRTSubrouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: mongoResourceRouter,
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
			ParentRouter: mongoResourceRouter,
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: mongoResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: linkrp.MongoDatabasesResourceType,
			Method:       mongo_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return mongo_ctrl.NewListSecretsMongoDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteRTSubrouter,
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
						RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
						ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
						RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
						ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: linkrp.DaprInvokeHttpRoutesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewDeleteDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprPubSubRTSubrouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: linkrp.DaprPubSubBrokersResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprSecretStoreRTSubrouter,
			ResourceType: linkrp.DaprSecretStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: daprStateStoreRTSubrouter,
			ResourceType: linkrp.DaprStateStoresResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: redisRTSubrouter,
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: redisResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: linkrp.RedisCachesResourceType,
			Method:       redis_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return redis_ctrl.NewListSecretsRedisCache(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqRTSubrouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewListResources(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationGet,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewGetResource(opt,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: rabbitmqResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: linkrp.RabbitMQMessageQueuesResourceType,
			Method:       rabbitmq_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		}, {
			ParentRouter: sqlRTSubrouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			ParentRouter: extenderRTSubrouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationList,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
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
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewCreateOrUpdateExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewCreateOrUpdateExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: linkrp.ExtendersResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewDeleteExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: linkrp.ExtendersResourceType,
			Method:       extender_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewListSecretsExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
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
