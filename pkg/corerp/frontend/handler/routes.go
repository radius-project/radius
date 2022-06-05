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

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
)

const (
	ProviderName = "Applications.Core"
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

	envRTSubrouter := subscriptionRt.PathPrefix("/resourcegroups/{resourceGroup}/providers/applications.core/environments").
		Queries(server.APIVersionParam, "{"+server.APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.Path("/{environment}").Subrouter()

	handerOptions := []server.HandlerOptions{
		{
			ParentRouter:   envRTSubrouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationList,
			HandlerFactory: env_ctrl.NewListEnvironments,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationGet,
			HandlerFactory: env_ctrl.NewGetEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPut,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationPatch,
			HandlerFactory: env_ctrl.NewCreateOrUpdateEnvironment,
		},
		{
			ParentRouter:   envResourceRouter,
			ResourceType:   env_ctrl.ResourceTypeName,
			Method:         v1.OperationDelete,
			HandlerFactory: env_ctrl.NewDeleteEnvironment,
		},
	}

	for _, h := range handerOptions {
		if err := server.RegisterHandler(ctx, sp, h); err != nil {
			return err
		}
	}

	return nil
}

/*

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	connector_provider "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/provider"

// AddConnectorRoutes adds routes and handlers for connector RP APIs.
func AddConnectorRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) error {
	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	// providerRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}").
	// 	Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	handlers := []handlerParam{}
	var resourceGroupLevelPath string

	if !hostoptions.IsSelfHosted() {
		// Tenant level API routes.
		tenantLevelPath := pathBase + "/providers/applications.connector"
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		operationsRouter := router.Path(tenantLevelPath+"/operations").
			Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
		h := []handlerParam{
			// TODO PUT subscriptions is a tenant-level api, implement it after connector RP is separated out of core RP.
			// Provider handler registration.
			// {providerRouter, connector_provider.ResourceTypeName, http.MethodPut, connectorRPPrefix + "subscription", connector_provider.NewCreateOrUpdateSubscription},
			// Provider operations.
			{operationsRouter, connector_provider.ResourceTypeName, http.MethodGet, asyncoperation.OperationGetOperations, connector_provider.NewGetOperations},
		}
		handlers = append(handlers, h...)
		// Resource Group level API routes.
		resourceGroupLevelPath = pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.connector"
	} else {
		// Resource Group level API routes.
		resourceGroupLevelPath = pathBase + "/resourcegroups/{resourceGroup}/providers/applications.connector"
	}

	// Adds mongo connector resource type routes
	mongoResourceTypeSubrouter := router.PathPrefix(resourceGroupLevelPath+"/mongodatabases").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	mongoResourceRouter := mongoResourceTypeSubrouter.Path("/{mongoDatabases}").Subrouter()
	mongoListSecretsRouter := mongoResourceRouter.Path("/listsecrets").Subrouter()

	h := []handlerParam{
		// MongoDatabases operations
		{mongoResourceTypeSubrouter, mongo_ctrl.ResourceTypeName, http.MethodGet, asyncoperation.OperationList, mongo_ctrl.NewListMongoDatabases},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodGet, asyncoperation.OperationGet, mongo_ctrl.NewGetMongoDatabase},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodPut, asyncoperation.OperationPut, mongo_ctrl.NewCreateOrUpdateMongoDatabase},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodDelete, asyncoperation.OperationDelete, mongo_ctrl.NewDeleteMongoDatabase},
		{mongoListSecretsRouter, mongo_ctrl.ResourceTypeName, http.MethodPost, mongo_ctrl.OperationListSecret, mongo_ctrl.NewListSecretsMongoDatabase},
	}
	handlers = append(handlers, h...)

	// Register handlers
	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.routeName, h.fn); err != nil {
			return fmt.Errorf("failed to register %s handler for route %s: %w", h.method, h.routeName, err)
		}
	}

	return nil
}
*/
