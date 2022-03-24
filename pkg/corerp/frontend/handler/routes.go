// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handler

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/frontend/controllers"
	"github.com/project-radius/radius/pkg/radrp/rest"
)

const (
	APIVersionParam = "api-version"
)

// AddRoutes adds the routes and handlers for each resource provider APIs.
// TODO: Enable api spec validator.
func AddRoutes(providerCtrl *controllers.ProviderController, rpCtrl *controllers.AppCoreController, router *mux.Router, validatorFactory ValidatorFactory, swaggerDocRoute string) {
	h := handler{
		providerCtrl:     providerCtrl,
		appCoreCtrl:      rpCtrl,
		validatorFactory: validatorFactory,
		pathPrefix:       swaggerDocRoute,
	}

	router.NotFoundHandler = http.HandlerFunc(notSupported)

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
	resourceGroupLevelPath := h.pathPrefix + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/applications.core/"

	// Adds environment resource type routes
	environmentRTSubrouter := router.Path(resourceGroupLevelPath+"/environments/{environment}").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	environmentRTSubrouter.Methods(http.MethodGet).HandlerFunc(h.ListEnvironments)
}

func notSupported(w http.ResponseWriter, req *http.Request) {
	response := rest.NewBadRequestResponse(fmt.Sprintf("Route not suported: %s", req.URL.Path))
	_ = response.Apply(req.Context(), w, req)
}
