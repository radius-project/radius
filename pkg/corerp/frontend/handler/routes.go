// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/frontend/controllers"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
)

const (
	APIVersionParam = "api-version"
)

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(db db.RadrpDB, deploy deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, swaggerDocRoute string) {
	providerCtrl := controllers.NewProviderController(db, deploy, nil, "http")
	appCoreCtrl := controllers.NewAppCoreController(db, deploy, nil, "http")

	h := handler{
		providerCtrl:     providerCtrl,
		appCoreCtrl:      appCoreCtrl,
		validatorFactory: validatorFactory,
		pathPrefix:       swaggerDocRoute,
	}

	// Provider system notification.
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/subscription-lifecycle-api-reference.md#creating-or-updating-a-subscription
	router.Path("/subscriptions/{subscriptionID}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").
		Methods(http.MethodPut).HandlerFunc(h.CreateOrUpdateSubscription)

	// Tenant level API routes.
	tenantLevelPath := h.pathPrefix + "/providers/applications.core"
	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").
		Methods(http.MethodGet).HandlerFunc(h.GetOperations)

	// Resource Group level API routes.
	resourceGroupLevelPath := h.pathPrefix + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core"

	// Adds environment resource type routes
	environmentRTSubrouter := router.Path(resourceGroupLevelPath+"/environments/{environment}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	environmentRTSubrouter.Methods(http.MethodGet).HandlerFunc(h.ListEnvironments)
}
