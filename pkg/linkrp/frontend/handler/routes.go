// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"time"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	frontend_ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	"github.com/project-radius/radius/pkg/linkrp/datamodel"
	"github.com/project-radius/radius/pkg/linkrp/datamodel/converter"
	link_frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
	daprHttpRoute_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprinvokehttproutes"
	daprPubSub_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprpubsubbrokers"
	daprSecretStore_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprsecretstores"
	daprStateStore_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/daprstatestores"
	extender_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/extenders"
	mongo_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/mongodatabases"
	rabbitmq_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rabbitmqmessagequeues"
	redis_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller/sqldatabases"
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
			ResourceType: mongo_ctrl.ResourceTypeName,
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
			ResourceType: mongo_ctrl.ResourceTypeName,
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
			ResourceType: mongo_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: time.Duration(10) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: mongo_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: time.Duration(8) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: mongo_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.MongoDatabase]{
							rp_frontend.PrepareRadiusResource[*datamodel.MongoDatabase],
						},
						AsyncOperationTimeout: time.Duration(15) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: mongo_ctrl.ResourceTypeName,
			Method:       mongo_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return mongo_ctrl.NewListSecretsMongoDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteRTSubrouter,
			ResourceType: daprHttpRoute_ctrl.ResourceTypeName,
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
			ResourceType: daprHttpRoute_ctrl.ResourceTypeName,
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
			ResourceType: daprHttpRoute_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: daprHttpRoute_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewCreateOrUpdateDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			}},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: daprHttpRoute_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprHttpRoute_ctrl.NewDeleteDaprInvokeHttpRoute(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			}},
		{
			ParentRouter: daprPubSubRTSubrouter,
			ResourceType: daprPubSub_ctrl.ResourceTypeName,
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
			ResourceType: daprPubSub_ctrl.ResourceTypeName,
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
			ResourceType: daprPubSub_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprPubSub_ctrl.NewCreateOrUpdateDaprPubSubBroker(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: daprPubSub_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprPubSub_ctrl.NewCreateOrUpdateDaprPubSubBroker(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: daprPubSub_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprPubSub_ctrl.NewDeleteDaprPubSubBroker(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprSecretStoreRTSubrouter,
			ResourceType: daprSecretStore_ctrl.ResourceTypeName,
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
			ResourceType: daprSecretStore_ctrl.ResourceTypeName,
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
			ResourceType: daprSecretStore_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprSecretStore_ctrl.NewCreateOrUpdateDaprSecretStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: daprSecretStore_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprSecretStore_ctrl.NewCreateOrUpdateDaprSecretStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: daprSecretStore_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprSecretStore_ctrl.NewDeleteDaprSecretStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprStateStoreRTSubrouter,
			ResourceType: daprStateStore_ctrl.ResourceTypeName,
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
			ResourceType: daprStateStore_ctrl.ResourceTypeName,
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
			ResourceType: daprStateStore_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprStateStore_ctrl.NewCreateOrUpdateDaprStateStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: daprStateStore_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprStateStore_ctrl.NewCreateOrUpdateDaprStateStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: daprStateStore_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return daprStateStore_ctrl.NewDeleteDaprStateStore(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
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
			ParentRouter: redisResourceRouter,
			ResourceType: redis_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RedisCache]{
							rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
						},
						AsyncOperationTimeout: time.Duration(10) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: redis_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncPut(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RedisCache]{
							rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
						},
						AsyncOperationTimeout: time.Duration(10) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: redis_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return defaultoperation.NewDefaultAsyncDelete(opt,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
						UpdateFilters: []frontend_ctrl.UpdateFilter[datamodel.RedisCache]{
							rp_frontend.PrepareRadiusResource[*datamodel.RedisCache],
						},
						AsyncOperationTimeout: time.Duration(10) * time.Minute,
					},
				)
			},
		},
		{
			ParentRouter: redisResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: redis_ctrl.ResourceTypeName,
			Method:       redis_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return redis_ctrl.NewListSecretsRedisCache(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqRTSubrouter,
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
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
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
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
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewCreateOrUpdateRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewCreateOrUpdateRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewDeleteRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: rabbitmq_ctrl.ResourceTypeName,
			Method:       rabbitmq_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		}, {
			ParentRouter: sqlRTSubrouter,
			ResourceType: sql_ctrl.ResourceTypeName,
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
			ResourceType: sql_ctrl.ResourceTypeName,
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
			ResourceType: sql_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return sql_ctrl.NewCreateOrUpdateSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: sql_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return sql_ctrl.NewCreateOrUpdateSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: sql_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return sql_ctrl.NewDeleteSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderRTSubrouter,
			ResourceType: extender_ctrl.ResourceTypeName,
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
			ResourceType: extender_ctrl.ResourceTypeName,
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
			ResourceType: extender_ctrl.ResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewCreateOrUpdateExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: extender_ctrl.ResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewCreateOrUpdateExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: extender_ctrl.ResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return extender_ctrl.NewDeleteExtender(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: extenderResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: extender_ctrl.ResourceTypeName,
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
