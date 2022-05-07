// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"

	env_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/environments"
	operation_status_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/operation/status"
	provider_ctrl "github.com/project-radius/radius/pkg/corerp/frontend/controller/provider"
)

const (
	APIVersionParam          = "api-version"
	serviceNamePrefix        = "corerp_"
	subscriptionRouteName    = serviceNamePrefix + "subscriptionAPI"
	operationsRouteName      = serviceNamePrefix + "operationsAPI"
	environmentRouteName     = serviceNamePrefix + "environmentAPI"
	operationResultRouteName = serviceNamePrefix + "operationResultAPI"
	operationStatusRouteName = serviceNamePrefix + "operationStatusAPI"
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

	// TODO: Make some of it constant like 'subscriptions/{subscriptionId}
	operationResultRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}/providers/Applications.Core/locations/{location}/operationResults/{operationId}").Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	operationStatusRouter := router.Path(pathBase+"/subscriptions/{subscriptionID}/providers/Applications.Core/locations/{location}/operationStatuses/{operationId}").Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()

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
		{envRTSubrouter, env_ctrl.ResourceTypeName, http.MethodGet, environmentRouteName, env_ctrl.NewListEnvironments},
		{envResourceRouter, env_ctrl.ResourceTypeName, http.MethodGet, environmentRouteName, env_ctrl.NewGetEnvironment},

		// OperationResult resource handler registration
		{operationResultRouter, env_ctrl.ResourceTypeName, http.MethodGet, environmentRouteName, env_ctrl.NewGetEnvironment},

		// OperationStatus resource handler registration
		{operationStatusRouter, operation_status_ctrl.ResourceTypeName, http.MethodGet, operationStatusRouteName, operation_status_ctrl.NewGetOperationStatus},
	}

	for _, h := range handlers {
		if err := registerHandler(ctx, sp, h.parent, h.resourcetype, h.method, h.routeName, h.fn); err != nil {
			return err
		}
	}

	return nil
}
