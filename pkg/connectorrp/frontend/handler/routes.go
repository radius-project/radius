// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	daprHttpRoute_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprinvokehttproutes"
	daprPubSub_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprpubsubbrokers"
	daprSecretStore_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprsecretstores"
	daprStateStore_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprstatestores"
	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/sqldatabases"
)

const (
	ProviderNamespaceName = "Applications.Connector"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, sm manager.StatusManager, router *mux.Router, pathBase string, isARM bool) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, sm, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	mongoRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/mongodatabases").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.PathPrefix("/{mongoDatabases}").Subrouter()

	daprHttpRouteRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/daprinvokehttproutes").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	daprHttpRouteResourceRouter := daprHttpRouteRTSubrouter.PathPrefix("/{daprInvokeHttpRoutes}").Subrouter()

	daprPubSubRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/daprpubsubbrokers").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	daprPubSubResourceRouter := daprPubSubRTSubrouter.PathPrefix("/{daprPubSubBrokers}").Subrouter()

	daprSecretStoreRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/daprsecretstores").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	daprSecretStoreResourceRouter := daprSecretStoreRTSubrouter.PathPrefix("/{daprSecretStores}").Subrouter()

	daprStateStoreRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/daprstatestores").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	daprStateStoreResourceRouter := daprStateStoreRTSubrouter.PathPrefix("/{daprStateStores}").Subrouter()

	redisRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/rediscaches").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	redisResourceRouter := redisRTSubrouter.PathPrefix("/{redisDatabases}").Subrouter()

	rabbitmqRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/rabbitmqmessagequeues").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	rabbitmqResourceRouter := rabbitmqRTSubrouter.PathPrefix("/{rabbitmqmessagequeues}").Subrouter()

	sqlRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/sqldatabases").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	sqlResourceRouter := sqlRTSubrouter.PathPrefix("/{sqlDatabases}").Subrouter()
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
			ParentRouter:   mongoResourceRouter.Path("/listsecrets").Subrouter(),
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
			ParentRouter:   redisRTSubrouter,
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: redis_ctrl.NewListRedisCaches,
		},
		{
			ParentRouter:   redisResourceRouter,
			ResourceType:   redis_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: redis_ctrl.NewGetRedisCache,
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
			ParentRouter:   redisResourceRouter.Path("/listsecrets").Subrouter(),
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
			ParentRouter:   rabbitmqResourceRouter.Path("/listsecrets").Subrouter(),
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
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, sm, h); err != nil {
			return err
		}
	}

	return nil
}
