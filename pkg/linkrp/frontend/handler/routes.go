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
	link_frontend_ctrl "github.com/project-radius/radius/pkg/linkrp/frontend/controller"
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
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.MongoDatabase]{
						RequestConverter:  converter.MongoDatabaseDataModelFromVersioned,
						ResponseConverter: converter.MongoDatabaseDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: mongoResourceRouter,
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return mongo_ctrl.NewDeleteMongoDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: mongoResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: link_frontend_ctrl.MongoDatabasesResourceTypeName,
			Method:       link_frontend_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return mongo_ctrl.NewListSecretsMongoDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: daprHttpRouteRTSubrouter,
			ResourceType: link_frontend_ctrl.DaprInvokeHttpRoutesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprInvokeHttpRoutesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprInvokeHttpRoutesResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
						RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
						ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: link_frontend_ctrl.DaprInvokeHttpRoutesResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
						RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
						ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			}},
		{
			ParentRouter: daprHttpRouteResourceRouter,
			ResourceType: link_frontend_ctrl.DaprInvokeHttpRoutesResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprInvokeHttpRoute]{
						RequestConverter:  converter.DaprInvokeHttpRouteDataModelFromVersioned,
						ResponseConverter: converter.DaprInvokeHttpRouteDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			}},
		{
			ParentRouter: daprPubSubRTSubrouter,
			ResourceType: link_frontend_ctrl.DaprPubSubBrokersResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprPubSubBrokersResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprPubSubBrokersResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: link_frontend_ctrl.DaprPubSubBrokersResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprPubSubResourceRouter,
			ResourceType: link_frontend_ctrl.DaprPubSubBrokersResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprPubSubBroker]{
						RequestConverter:  converter.DaprPubSubBrokerDataModelFromVersioned,
						ResponseConverter: converter.DaprPubSubBrokerDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: daprSecretStoreRTSubrouter,
			ResourceType: link_frontend_ctrl.DaprSecretStoresResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprSecretStoresResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprSecretStoresResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: link_frontend_ctrl.DaprSecretStoresResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprSecretStoreResourceRouter,
			ResourceType: link_frontend_ctrl.DaprSecretStoresResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprSecretStore]{
						RequestConverter:  converter.DaprSecretStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprSecretStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: daprStateStoreRTSubrouter,
			ResourceType: link_frontend_ctrl.DaprStateStoresResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprStateStoresResourceTypeName,
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
			ResourceType: link_frontend_ctrl.DaprStateStoresResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: link_frontend_ctrl.DaprStateStoresResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: daprStateStoreResourceRouter,
			ResourceType: link_frontend_ctrl.DaprStateStoresResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.DaprStateStore]{
						RequestConverter:  converter.DaprStateStoreDataModelFromVersioned,
						ResponseConverter: converter.DaprStateStoreDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: redisRTSubrouter,
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: redisResourceRouter,
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RedisCache]{
						RequestConverter:  converter.RedisCacheDataModelFromVersioned,
						ResponseConverter: converter.RedisCacheDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: redisResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: link_frontend_ctrl.RedisCachesResourceTypeName,
			Method:       link_frontend_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return redis_ctrl.NewListSecretsRedisCache(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: rabbitmqRTSubrouter,
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter,
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.RabbitMQMessageQueue]{
						RequestConverter:  converter.RabbitMQMessageQueueDataModelFromVersioned,
						ResponseConverter: converter.RabbitMQMessageQueueDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: rabbitmqResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: link_frontend_ctrl.RabbitMQMessageQueuesResourceTypeName,
			Method:       link_frontend_ctrl.OperationListSecret,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return rabbitmq_ctrl.NewListSecretsRabbitMQMessageQueue(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		}, {
			ParentRouter: sqlRTSubrouter,
			ResourceType: link_frontend_ctrl.SqlDatabasesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.SqlDatabasesResourceTypeName,
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
			ResourceType: link_frontend_ctrl.SqlDatabasesResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: link_frontend_ctrl.SqlDatabasesResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: link_frontend_ctrl.SqlDatabasesResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.SqlDatabase]{
						RequestConverter:  converter.SqlDatabaseDataModelFromVersioned,
						ResponseConverter: converter.SqlDatabaseDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: extenderRTSubrouter,
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
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
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
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
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
			Method:       v1.OperationPut,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
					})
				return link_frontend_ctrl.NewCreateOrUpdateResource(
					options,
					operation,
					false,
				)
			},
		},
		{
			ParentRouter: extenderResourceRouter,
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				options := link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp}
				operation := frontend_ctrl.NewOperation(options.Options,
					frontend_ctrl.ResourceOptions[datamodel.Extender]{
						RequestConverter:  converter.ExtenderDataModelFromVersioned,
						ResponseConverter: converter.ExtenderDataModelToVersioned,
					})
				return link_frontend_ctrl.NewDeleteResource(
					options,
					operation,
				)
			},
		},
		{
			ParentRouter: extenderResourceRouter.PathPrefix("/listsecrets").Subrouter(),
			ResourceType: link_frontend_ctrl.ExtendersResourceTypeName,
			Method:       link_frontend_ctrl.OperationListSecret,
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
