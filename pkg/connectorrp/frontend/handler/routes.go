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

	daprHttpRoute_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprinvokehttproutes"
	daprPubSub_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprpubsubbrokers"
	daprSecretStore_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprsecretstores"
	daprStateStore_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/daprstatestores"
	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
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
	}

	for _, h := range handlerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}
