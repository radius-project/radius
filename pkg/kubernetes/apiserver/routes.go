// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"fmt"
	"net/http"

	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/radrp/schema"
	"github.com/gorilla/mux"
)

func AddRoutes(rp resourceprovider.ResourceProvider, router *mux.Router, validatorFactory ValidatorFactory) {
	// Nothing for now

	h := handler{rp: rp, validatorFactory: validatorFactory}
	var subrouter *mux.Router

	// when you register a crd, basically the api server will be responible for that group version
	// when you register an api service extension, I'm responsible for it
	// when you register the api version, you respond to the api server
	// discovery is part of the protocol, api server pings api service, what crds do you own?
	// don't need provide data, but need to respond to request, ready checks will fail, log what urls
	// see example url, hit different one, you get an example.

	// kubectl proxy

	// use kubeclient to create httpclient, don't create a separate public endpoint
	var providerPath = fmt.Sprintf(
		"/apis/api.radius.dev/v1alpha3/subscriptions/{%s}/resourceGroups/{%s}/providers/Microsoft.CustomProviders/resourceProviders/radiusv3",
		azresources.SubscriptionIDKey,
		azresources.ResourceGroupKey)
	router.Path("/apis/api.radius.dev/v1alpha3").Methods("GET").HandlerFunc(h.EmptySwaggerDoc)
	// router.Path("/apis/api.radius.dev").Methods("GET", "PUT", "DELETE", "POST").HandlerFunc(h.EmptySwaggerDoc)
	router.Path(fmt.Sprintf("%s/listSecrets", providerPath)).Methods("POST").HandlerFunc(h.ListSecrets)

	var applicationCollectionPath = fmt.Sprintf("%s/Application", providerPath)
	var applicationItemPath = fmt.Sprintf("%s/{%s}", applicationCollectionPath, azresources.ApplicationNameKey)

	var resourceCollectionPath = fmt.Sprintf("%s/{%s}", applicationItemPath, azresources.ResourceTypeKey)
	var resourceItemPath = fmt.Sprintf("%s/{%s}", resourceCollectionPath, azresources.ResourceNameKey)
	var operationItemPath = fmt.Sprintf("%s/{%s}/{%s}", resourceItemPath, "OperationResults", azresources.OperationIDKey)

	var allResourceCollectionPath = fmt.Sprintf("%s/%s", applicationItemPath, schema.GenericResourceType)
	var allResourceItemPath = fmt.Sprintf("%s/{%s}", allResourceCollectionPath, azresources.ResourceNameKey)

	router.Path(applicationCollectionPath).Methods("GET").HandlerFunc(h.ListApplications)
	subrouter = router.Path(applicationItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetApplication)
	subrouter.Methods("PUT").HandlerFunc(h.UpdateApplication)
	subrouter.Methods("DELETE").HandlerFunc(h.DeleteApplication)

	router.Path(allResourceCollectionPath).Methods("GET").HandlerFunc(h.ListAllV3ResourcesByApplication)
	router.Path(allResourceItemPath).HandlerFunc(notSupported)

	router.Path(resourceCollectionPath).Methods("GET").HandlerFunc(h.ListResources)
	subrouter = router.Path(resourceItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetResource)
	subrouter.Methods("PUT").HandlerFunc(h.UpdateResource)
	subrouter.Methods("DELETE").HandlerFunc(h.DeleteResource)

	subrouter = router.Path(operationItemPath).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.GetOperation)
}

func notSupported(w http.ResponseWriter, req *http.Request) {
	response := rest.NewBadRequestResponse(fmt.Sprintf("Route not suported: %s", req.URL.Path))
	_ = response.Apply(req.Context(), w, req)
}
