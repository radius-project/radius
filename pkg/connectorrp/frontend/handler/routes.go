// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"

	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	sql_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/sqldatabases"
)

const (
	ProviderNamespaceName = "Applications.Connector"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, router *mux.Router, pathBase string, isARM bool) error {
	if isARM {
		pathBase += "/subscriptions/{subscriptionID}"
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, router, pathBase, isARM, ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	mongoRTSubrouter := router.NewRoute().PathPrefix(pathBase+"/resourcegroups/{resourceGroup}/providers/applications.connector/mongodatabases").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.PathPrefix("/{mongoDatabases}").Subrouter()

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
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
