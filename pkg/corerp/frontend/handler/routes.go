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
	"github.com/project-radius/radius/pkg/corerp/asyncoperation"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"

	mongo_ctrl "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/mongodatabases"
	connector_provider "github.com/project-radius/radius/pkg/connectorrp/frontend/controller/provider"
)

const (
	APIVersionParam = "api-version"
)

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(ctx context.Context, sp dataprovider.DataStorageProvider, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) error {
	var resourceGroupLevelPath string
	var providerRouter *mux.Router
	var operationsRouter *mux.Router
	handlers := []handlerParam{}

	if !hostoptions.IsSelfHosted() {
		// Provider system notification.
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
		providerRouter = router.Path(pathBase+"/subscriptions/{subscriptionID}").
			Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

		// Tenant level API routes.
		tenantLevelPath := pathBase + "/providers/applications.core"
		// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
		operationsRouter = router.Path(tenantLevelPath+"/operations").
			Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

		// OperationStatus resource paths
		locationLevelPath := pathBase + "/subscriptions/{subscriptionID}/providers/applications.core/locations/{location}"
		locationsRouter := router.PathPrefix(locationLevelPath).
			Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

		operationStatusRouter := locationsRouter.Path("/operationstatuses/{operationId}").Subrouter()

		// OperationResult resource paths
		operationResultRouter := locationsRouter.Path("/operationresults/{operationId}").Subrouter()

		// Resource Group level API routes.
		resourceGroupLevelPath = pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core"

		h := []handlerParam{
			// Provider handler registration.
			{providerRouter, provider_ctrl.ResourceTypeName, http.MethodPut, asyncoperation.OperationPutSubscriptions, provider_ctrl.NewCreateOrUpdateSubscription},
			{operationsRouter, provider_ctrl.ResourceTypeName, http.MethodGet, asyncoperation.OperationGetOperations, provider_ctrl.NewGetOperations},
			{operationStatusRouter, provider_ctrl.OperationStatusResourceTypeName, http.MethodGet, asyncoperation.OperationGetOperationStatuses, provider_ctrl.NewGetOperationStatus},
			{operationResultRouter, provider_ctrl.OperationStatusResourceTypeName, http.MethodGet, asyncoperation.OperationGetOperationResult, provider_ctrl.NewGetOperationResult},
		}
		handlers = append(handlers, h...)

	} else {
		resourceGroupLevelPath = pathBase + "/resourcegroups/{resourceGroup}/providers/applications.core"
	}

	// Adds environment resource type routes
	envRTSubrouter := router.PathPrefix(resourceGroupLevelPath+"/environments").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	envResourceRouter := envRTSubrouter.Path("/{environment}").Subrouter()

	h := []handlerParam{
		// Environments resource handler registration.
		{envRTSubrouter, env_ctrl.ResourceTypeName, http.MethodGet, asyncoperation.OperationList, env_ctrl.NewListEnvironments},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodGet, asyncoperation.OperationGet, env_ctrl.NewGetEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPut, asyncoperation.OperationPut, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodPatch, asyncoperation.OperationPatch, env_ctrl.NewCreateOrUpdateEnvironment},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodDelete, asyncoperation.OperationDelete, env_ctrl.NewDeleteEnvironment},
	}
	handlers = append(handlers, h...)

	// Create the operational controller and add new resource types' handlers.
	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.routeName, h.fn); err != nil {
			return err
		}
	}

	return nil
}

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
	mongoResourceTypeSubrouter := router.PathPrefix(resourceGroupLevelPath+"/mongoDatabases").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

	mongoResourceRouter := mongoResourceTypeSubrouter.Path("/{mongoDatabases}").Subrouter()
	mongoListSecretsRouter := mongoResourceRouter.Path("/listSecrets").Subrouter()

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
