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
	"github.com/project-radius/radius/pkg/linkrp"
	rp_frontend "github.com/project-radius/radius/pkg/rp/frontend"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	"github.com/project-radius/radius/pkg/datastoresrp/datamodel"
	"github.com/project-radius/radius/pkg/datastoresrp/datamodel/converter"
	link_frontend_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller"
	mongo_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller/mongodatabases"
	redis_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller/rediscaches"
	sql_ctrl "github.com/project-radius/radius/pkg/datastoresrp/frontend/controller/sqldatabases"
	"github.com/project-radius/radius/pkg/linkrp/frontend/deployment"
)

const (
	DatastoresProviderNamespace = "Applications.Datastores"
)

func AddDatastoresRoutes(ctx context.Context, router *mux.Router, pathBase string, isARM bool, ctrlOpts frontend_ctrl.Options, dp deployment.DeploymentProcessor) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	} else {
		pathBase += "/planes/radius/{planeName}"
	}
	resourceGroupPath := "/resourcegroups/{resourceGroupName}"

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, router, pathBase, isARM, DatastoresProviderNamespace, NewGetOperations, ctrlOpts)
	if err != nil {
		return err
	}

	specLoader, err := validator.LoadSpec(ctx, DatastoresProviderNamespace, swagger.SpecFiles, pathBase+resourceGroupPath, "rootScope")
	if err != nil {
		return err
	}

	rootScopeRouter := router.PathPrefix(pathBase + resourceGroupPath).Subrouter()
	rootScopeRouter.Use(validator.APIValidator(specLoader))

	mongoRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.datastores/mongodatabases").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.PathPrefix("/{mongoDatabaseName}").Subrouter()

	redisRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.datastores/rediscaches").Subrouter()
	redisResourceRouter := redisRTSubrouter.PathPrefix("/{redisCacheName}").Subrouter()

	sqlRTSubrouter := rootScopeRouter.PathPrefix("/providers/applications.datastores/sqldatabases").Subrouter()
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
				return sql_ctrl.NewCreateOrUpdateSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationPatch,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return sql_ctrl.NewCreateOrUpdateSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
			},
		},
		{
			ParentRouter: sqlResourceRouter,
			ResourceType: linkrp.SqlDatabasesResourceType,
			Method:       v1.OperationDelete,
			HandlerFactory: func(opt frontend_ctrl.Options) (frontend_ctrl.Controller, error) {
				return sql_ctrl.NewDeleteSqlDatabase(link_frontend_ctrl.Options{Options: opt, DeployProcessor: dp})
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
