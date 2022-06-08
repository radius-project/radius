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
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
)

const (
	ProviderNamespaceName = "Applications.Connector"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, router *mux.Router, pathBase string) error {
	root := router
	if pathBase != "" {
		root = router.PathPrefix(pathBase).Subrouter()
	}

	scopeRoute := root
	if !hostoptions.IsSelfHosted() {
		scopeRoute = root.PathPrefix("/subscriptions/{subscriptionID}").Subrouter()
	} else {
		// TODO: Enable ucp path.
		// scopeRoute = root.PathPrefix("/planes/radius/{radiusTenant}").Subrouter()
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, root, scopeRoute, !hostoptions.IsSelfHosted(), ProviderNamespaceName, NewGetOperations)
	if err != nil {
		return err
	}

	mongoRTSubrouter := scopeRoute.PathPrefix("/resourcegroups/{resourceGroup}/providers/applications.connector/mongodatabases").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.PathPrefix("/{mongoDatabases}").Subrouter()
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
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
