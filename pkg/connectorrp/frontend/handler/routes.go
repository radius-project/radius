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
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
)

const (
	ProviderName = "Applications.Connector"
)

func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, router *mux.Router, pathBase string) error {
	root := router.Path(pathBase).Subrouter()
	var subscriptionRt *mux.Router

	if !hostoptions.IsSelfHosted() {
		subscriptionRt = router.Path(pathBase + "/subscriptions/{subscriptionID}").Subrouter()
	} else {
		subscriptionRt = router.Path(pathBase + "/planes/radius/{radiusTenant}").Subrouter()
	}

	// Configure the default ARM handlers.
	err := server.ConfigureDefaultHandlers(ctx, sp, root, subscriptionRt, ProviderName, nil)
	if err != nil {
		return err
	}

	mongoRTSubrouter := subscriptionRt.PathPrefix("/resourcegroups/{resourceGroup}/providers/applications.connector/mongodatabases").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	mongoResourceRouter := mongoRTSubrouter.Path("/{mongoDatabases}").Subrouter()

	handerOptions := []server.HandlerOptions{
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
	}

	for _, h := range handerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
