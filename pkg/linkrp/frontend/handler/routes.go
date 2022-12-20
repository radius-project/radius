// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	daprHttpRoute_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprinvokehttproutes"
	daprPubSub_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprpubsubbrokers"
	daprSecretStore_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprsecretstores"
	daprStateStore_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprstatestores"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
)

const (
	ProviderNamespaceName = "Applications.Link"
)

func AddRoutes(ctx context.Context, router *mux.Router, pathBase string, isARM bool, ctrlOpts frontend_ctrl.Options) error {
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
			ParentRouter:   mongoRTSubrouter,
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: mongo_ctrl.NewListMongoDatabases,
		},
		{
			ParentRouter:   mongoResourceRouter,
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: mongo_ctrl.NewGetMongoDatabase,
		},
		{
			ParentRouter:   mongoResourceRouter,
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: mongo_ctrl.NewCreateOrUpdateMongoDatabase,
		},
		{
			ParentRouter:   mongoResourceRouter,
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: mongo_ctrl.NewCreateOrUpdateMongoDatabase,
		},
		{
			ParentRouter:   mongoResourceRouter,
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: mongo_ctrl.NewDeleteMongoDatabase,
		},
		{
			ParentRouter:   mongoResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   mongo_ctrl.ResourceTypeName,
			Method:         mongo_ctrl.OperationListSecret,
			HandlerFactory: mongo_ctrl.NewListSecretsMongoDatabase,
		},
		{
			ParentRouter:   daprHttpRouteRTSubrouter,
			ResourceType:   daprHttpRoute_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: daprHttpRoute_ctrl.NewListDaprInvokeHttpRoutes,
		},
		{
			ParentRouter:   daprHttpRouteResourceRouter,
			ResourceType:   daprHttpRoute_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: daprHttpRoute_ctrl.NewGetDaprInvokeHttpRoute,
		},
		{
			ParentRouter:   daprHttpRouteResourceRouter,
			ResourceType:   daprHttpRoute_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute,
		},
		{
			ParentRouter:   daprHttpRouteResourceRouter,
			ResourceType:   daprHttpRoute_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute,
		},
		{
			ParentRouter:   daprHttpRouteResourceRouter,
			ResourceType:   daprHttpRoute_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: daprHttpRoute_ctrl.NewDeleteDaprInvokeHttpRoute,
		},
		{
			ParentRouter:   daprPubSubRTSubrouter,
			ResourceType:   daprPubSub_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: daprPubSub_ctrl.NewListDaprPubSubBrokers,
		},
		{
			ParentRouter:   daprPubSubResourceRouter,
			ResourceType:   daprPubSub_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: daprPubSub_ctrl.NewGetDaprPubSubBroker,
		},
		{
			ParentRouter:   daprPubSubResourceRouter,
			ResourceType:   daprPubSub_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: daprPubSub_ctrl.NewCreateOrUpdateDaprPubSubBroker,
		},
		{
			ParentRouter:   daprPubSubResourceRouter,
			ResourceType:   daprPubSub_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: daprPubSub_ctrl.NewCreateOrUpdateDaprPubSubBroker,
		},
		{
			ParentRouter:   daprPubSubResourceRouter,
			ResourceType:   daprPubSub_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: daprPubSub_ctrl.NewDeleteDaprPubSubBroker,
		},
		{
			ParentRouter:   daprSecretStoreRTSubrouter,
			ResourceType:   daprSecretStore_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: daprSecretStore_ctrl.NewListDaprSecretStores,
		},
		{
			ParentRouter:   daprSecretStoreResourceRouter,
			ResourceType:   daprSecretStore_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: daprSecretStore_ctrl.NewGetDaprSecretStore,
		},
		{
			ParentRouter:   daprSecretStoreResourceRouter,
			ResourceType:   daprSecretStore_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: daprSecretStore_ctrl.NewCreateOrUpdateDaprSecretStore,
		},
		{
			ParentRouter:   daprSecretStoreResourceRouter,
			ResourceType:   daprSecretStore_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: daprSecretStore_ctrl.NewCreateOrUpdateDaprSecretStore,
		},
		{
			ParentRouter:   daprSecretStoreResourceRouter,
			ResourceType:   daprSecretStore_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: daprSecretStore_ctrl.NewDeleteDaprSecretStore,
		},
		{
			ParentRouter:   daprStateStoreRTSubrouter,
			ResourceType:   daprStateStore_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: daprStateStore_ctrl.NewListDaprStateStores,
		},
		{
			ParentRouter:   daprStateStoreResourceRouter,
			ResourceType:   daprStateStore_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: daprStateStore_ctrl.NewGetDaprStateStore,
		},
		{
			ParentRouter:   daprStateStoreResourceRouter,
			ResourceType:   daprStateStore_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: daprStateStore_ctrl.NewCreateOrUpdateDaprStateStore,
		},
		{
			ParentRouter:   daprStateStoreResourceRouter,
			ResourceType:   daprStateStore_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: daprStateStore_ctrl.NewCreateOrUpdateDaprStateStore,
		},
		{
			ParentRouter:   daprStateStoreResourceRouter,
			ResourceType:   daprStateStore_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: daprStateStore_ctrl.NewDeleteDaprStateStore,
		},
		{
			ParentRouter: redisRTSubrouter,
			ResourceType: redis_ctrl.ResourceTypeName,
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
			ResourceType: redis_ctrl.ResourceTypeName,
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
			ParentRouter:   redisResourceRouter,
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: redis_ctrl.NewCreateOrUpdateRedisCache,
		},
		{
			ParentRouter:   redisResourceRouter,
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: redis_ctrl.NewCreateOrUpdateRedisCache,
		},
		{
			ParentRouter:   redisResourceRouter,
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: redis_ctrl.NewDeleteRedisCache,
		},
		{
			ParentRouter:   redisResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         redis_ctrl.OperationListSecret,
			HandlerFactory: redis_ctrl.NewListSecretsRedisCache,
		},
		{
			ParentRouter:   rabbitmqRTSubrouter,
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: rabbitmq_ctrl.NewListRabbitMQMessageQueues,
		},
		{
			ParentRouter:   rabbitmqResourceRouter,
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: rabbitmq_ctrl.NewGetRabbitMQMessageQueue,
		},
		{
			ParentRouter:   rabbitmqResourceRouter,
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: rabbitmq_ctrl.NewCreateOrUpdateRabbitMQMessageQueue,
		},
		{
			ParentRouter:   rabbitmqResourceRouter,
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: rabbitmq_ctrl.NewCreateOrUpdateRabbitMQMessageQueue,
		},
		{
			ParentRouter:   rabbitmqResourceRouter,
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: rabbitmq_ctrl.NewDeleteRabbitMQMessageQueue,
		},
		{
			ParentRouter:   rabbitmqResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   rabbitmq_ctrl.ResourceTypeName,
			Method:         rabbitmq_ctrl.OperationListSecret,
			HandlerFactory: rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue,
		}, {
			ParentRouter:   sqlRTSubrouter,
			ResourceType:   sql_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: sql_ctrl.NewListSqlDatabases,
		},
		{
			ParentRouter:   sqlResourceRouter,
			ResourceType:   sql_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: sql_ctrl.NewGetSqlDatabase,
		},
		{
			ParentRouter:   sqlResourceRouter,
			ResourceType:   sql_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: sql_ctrl.NewCreateOrUpdateSqlDatabase,
		},
		{
			ParentRouter:   sqlResourceRouter,
			ResourceType:   sql_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: sql_ctrl.NewCreateOrUpdateSqlDatabase,
		},
		{
			ParentRouter:   sqlResourceRouter,
			ResourceType:   sql_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: sql_ctrl.NewDeleteSqlDatabase,
		},
		{
			ParentRouter:   extenderRTSubrouter,
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: extender_ctrl.NewListExtenders,
		},
		{
			ParentRouter:   extenderResourceRouter,
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: extender_ctrl.NewGetExtender,
		},
		{
			ParentRouter:   extenderResourceRouter,
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: extender_ctrl.NewCreateOrUpdateExtender,
		},
		{
			ParentRouter:   extenderResourceRouter,
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: extender_ctrl.NewCreateOrUpdateExtender,
		},
		{
			ParentRouter:   extenderResourceRouter,
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: extender_ctrl.NewDeleteExtender,
		},
		{
			ParentRouter:   extenderResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType:   extender_ctrl.ResourceTypeName,
			Method:         extender_ctrl.OperationListSecret,
			HandlerFactory: extender_ctrl.NewListSecretsExtender,
		},
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, h, ctrlOpts); err != nil {
			return err
		}
	}

	return nil
}
