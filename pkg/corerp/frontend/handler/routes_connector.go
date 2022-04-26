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

// AddConnectorRoutes adds routes and handlers for each connector RP APIs.
func AddConnectorRoutes(db db.RadrpDB, jobEngine deployment.DeploymentProcessor, router *mux.Router, validatorFactory ValidatorFactory, pathBase string) {
	h := handlerConnector{
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
	tenantLevelPath := h.pathBase + "/providers/Applications.Connector"

	// https://github.com/Azure/azure-resource-manager-rpc/blob/master/v1.0/proxy-api-reference.md#exposing-available-operations
	router.Path(tenantLevelPath+"/operations").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").
		Methods(http.MethodGet).HandlerFunc(h.getOperations)

	// Resource Group level API routes.
	resourceGroupLevelPath := h.pathBase + "/subscriptions/{subscriptionID}/resourcegroups/{resourceGroup}/providers/Applications.Connector"

	// Adds mongoDatabase resource type routes
	mongoDBSubrouter := router.PathPrefix(resourceGroupLevelPath+"/mongoDatabases").
		Queries(APIVersionParam, "{"+APIVersionParam+"}").Subrouter()
	mongoDBSubrouter.Path("/").Methods(http.MethodGet).HandlerFunc(h.listMongoDatabases)
	mongoDBResourceRouter := mongoDBSubrouter.Path("/{mongoDatabases}").Subrouter()
	mongoDBResourceRouter.Methods(http.MethodGet).HandlerFunc(h.getMongoDatabase)
	mongoDBResourceRouter.Methods(http.MethodPut).HandlerFunc(h.createOrUpdateMongoDatabase)
	mongoDBResourceRouter.Methods(http.MethodPatch).HandlerFunc(h.createOrUpdateMongoDatabase)
	mongoDBResourceRouter.Methods(http.MethodDelete).HandlerFunc(h.deleteMongoDatabase)
}
