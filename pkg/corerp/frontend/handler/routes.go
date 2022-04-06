// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
)

const (
	APIVersionParam = "api-version"
)

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(db db.RadrpDB, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) {
	h := handler{
		db:        db,
		jobEngine: jobEngine,
		pathBase:  pathBase,
	}

	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	router.Path(h.pathBase+"/subscriptions/{subscriptionID}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").
		Methods(http.MethodPut).HandlerFunc(h.createOrUpdateSubscription)

	// Tenant level API routes.
	tenantLevelPath := h.pathBase + "/providers/applications.core"
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").
		Methods(http.MethodGet).HandlerFunc(h.getOperations)

	// Resource Group level API routes.
	resourceGroupLevelPath := h.pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core"

	// Adds environment resource type routes
	environmentRTSubrouter := router.Path(resourceGroupLevelPath+"/environments/{environment}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	environmentRTSubrouter.Methods(http.MethodGet).HandlerFunc(h.createOrUpdateEnvironments)
}
