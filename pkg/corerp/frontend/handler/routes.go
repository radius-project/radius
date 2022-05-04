// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	connector_provider "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/provider"
)

const (
	APIVersionParam       = "api-version"
	serviceNamePrefix     = "corerp_"
	subscriptionRouteName = serviceNamePrefix + "subscriptionAPI"
	operationsRouteName   = serviceNamePrefix + "operationsAPI"
	environmentRouteName  = serviceNamePrefix + "environmentAPI"

	// Connector RP
	connectorRPPrefix              = "connectorrp_"
	connectorSubscriptionRouteName = connectorRPPrefix + "subscription"
	connectorOperationsRouteName   = connectorRPPrefix + "operations"
	mongoDatabaseRouteName         = connectorRPPrefix + "mongodatabase"
)

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) error {
	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	providerRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Tenant level API routes.
	tenantLevelPath := pathBase + "/providers/applications.core"
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	operationsRouter := router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Resource Group level API routes.
	resourceGroupLevelPath := pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core"

	// Adds environment resource type routes
	envRTSubrouter := router.PathPrefix(resourceGroupLevelPath+"/environments").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.Path("/{environment}").Subrouter()

	// Register handlers
	handlers := []handlerParam{
		// Provider handler registration.
		{providerRouter, provider_ctrl.ResourceTypeName, http.MethodPut, subscriptionRouteName, provider_ctrl.NewCreateOrUpdateSubscription},
		{operationsRouter, provider_ctrl.ResourceTypeName, http.MethodGet, operationsRouteName, provider_ctrl.NewGetOperations},
		// Environments resource handler registration.
		{envRTSubrouter, env_ctrl.ResourceTypeName, http.MethodGet, environmentRouteName, env_ctrl.NewListEnvironments},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodGet, environmentRouteName, env_ctrl.NewGetEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPut, environmentRouteName, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPatch, environmentRouteName, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodDelete, environmentRouteName, env_ctrl.NewDeleteEnvironment},

		// Create the operational controller and add new resource types' handlers.
	}

	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.routeName, h.fn); err != nil {
			return err
		}
	}

	if err := AddConnectorRoutes(ctx, sp, nil, router, validatorFactory, ""); err != nil {
		return err
	}

	return nil
}

// AddConnectorRoutes adds routes and handlers for connector RP APIs.
func AddConnectorRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) error {
	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	providerRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Tenant level API routes.
	tenantLevelPath := pathBase + "/providers/applications.connector"
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	operationsRouter := router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	// Resource Group level API routes.
	resourceGroupLevelPath := pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.connector"

	// Adds mongo connector resource type routes
	mongoResourceTypeSubrouter := router.PathPrefix(resourceGroupLevelPath+"/mongoDatabases").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	mongoResourceRouter := mongoResourceTypeSubrouter.Path("/{mongoDatabases}").Subrouter()

	// Register handlers
	handlers := []handlerParam{
		// Provider handler registration.
		{providerRouter, connector_provider.ResourceTypeName, http.MethodPut, connectorSubscriptionRouteName, connector_provider.NewCreateOrUpdateSubscription},
		// Provider operations.
		{operationsRouter, connector_provider.ResourceTypeName, http.MethodGet, connectorOperationsRouteName, connector_provider.NewGetOperations},
		// MongoDatabases operations
		{mongoResourceTypeSubrouter, mongo_ctrl.ResourceTypeName, http.MethodGet, mongoDatabaseRouteName, mongo_ctrl.NewListMongoDatabases},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodGet, mongoDatabaseRouteName, mongo_ctrl.NewGetMongoDatabase},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodPut, mongoDatabaseRouteName, mongo_ctrl.NewCreateOrUpdateMongoDatabase},
		{mongoResourceRouter, mongo_ctrl.ResourceTypeName, http.MethodDelete, mongoDatabaseRouteName, mongo_ctrl.NewDeleteMongoDatabase},
	}

	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.routeName, h.fn); err != nil {
			return fmt.Errorf("failed to register %s handler for route %s: %w", h.method, h.routeName, err)
		}
	}

	return nil
}
