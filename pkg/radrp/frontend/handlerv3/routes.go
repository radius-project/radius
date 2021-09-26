// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlerv3

import (
	"fmt"
	"net/http"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schemav3"
	"github.com/gorilla/mux"
)

func AddRoutes(rp resourceproviderv3.ResourceProvider, router *mux.Router, validatorFactory ValidatorFactory) {
	// Nothing for now

	h := handler{rp: rp, validatorFactory: validatorFactory}
	var subrouter *mux.Router

	var applicationCollectionPath = fmt.Sprintf(
		"/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application",
		azresources.SubscriptionIDKey,
		azresources.ResourceGroupKey)
	var applicationItemPath = fmt.Sprintf("%s/{%s}", applicationCollectionPath, azresources.ApplicationNameKey)

	var resourceCollectionPath = fmt.Sprintf("%s/{%s}", applicationItemPath, azresources.ResourceTypeKey)
	var resourceItemPath = fmt.Sprintf("%s/{%s}", resourceCollectionPath, azresources.ResourceNameKey)
	var operationItemPath = fmt.Sprintf("%s/{%s}/{%s}", resourceItemPath, "OperationResults", azresources.OperationIDKey)

	var allResourceCollectionPath = fmt.Sprintf("%s/%s", applicationItemPath, schemav3.GenericResourceType)
	var allResourceItemPath = fmt.Sprintf("%s/{%s}", allResourceCollectionPath, azresources.ResourceNameKey)

	router.Path(applicationCollectionPath).Methods("GET").HandlerFunc(h.ListApplications)
	subrouter = router.Path(applicationItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetApplication)
	subrouter.Methods("PUT").HandlerFunc(h.UpdateApplication)
	subrouter.Methods("DELETE").HandlerFunc(h.DeleteApplication)

	router.Path(allResourceCollectionPath).Methods("GET").HandlerFunc(h.ListAllV3ResourcesByApplication)
	subrouter.Path(allResourceItemPath).HandlerFunc(notSupported)

	router.Path(resourceCollectionPath).Methods("GET").HandlerFunc(h.ListResources)
	subrouter = router.Path(resourceItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetResource)
	subrouter.Methods("PUT").HandlerFunc(h.UpdateResource)
	subrouter.Methods("DELETE").HandlerFunc(h.DeleteResource)

	subrouter = router.Path(operationItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetOperation)
}

func notSupported(w http.ResponseWriter, req *http.Request) {
	response := rest.NewBadRequestResponse(fmt.Sprintf("Route not suported: %v", req.URL.Path))
	_ = response.Apply(req.Context(), w, req)
}
