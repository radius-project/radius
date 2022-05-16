// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/store"
)

//TODO: Use variables and construct the path as we add more APIs.
const (
	planeCollectionPath   = "/planes"
	planeItemPath         = "/planes/{PlaneType}/{PlaneID}"
	planeCollectionByType = "/planes/{PlaneType}"
)

var resourceGroupCollectionPath = fmt.Sprintf("%s/%s", planeItemPath, "resource{[gG]}roups")
var resourceGroupItemPath = fmt.Sprintf("%s/%s", resourceGroupCollectionPath, "{ResourceGroup}")

func Register(router *mux.Router, client store.StorageClient, ucp ucphandler.UCPHandler) {
	h := Handler{
		db:  client,
		ucp: ucp,
	}
	baseURL := ucp.Options.BasePath
	if baseURL != "" {
		router.Path(baseURL).Methods("GET").HandlerFunc(h.GetSwaggerDoc)
	}

	// TODO: Handle trailing slashes for matching routes
	// https://github.com/project-radius/radius/issues/2303
	var subrouter *mux.Router
	p := fmt.Sprintf("%s%s", baseURL, planeCollectionPath)
	router.Path(p).Methods(http.MethodGet).HandlerFunc(h.ListPlanes)
	p = fmt.Sprintf("%s%s", baseURL, planeCollectionByType)
	subrouter = router.Path(p).Subrouter()
	subrouter.Methods(http.MethodGet).HandlerFunc(h.ListPlanes)
	p = fmt.Sprintf("%s%s", baseURL, planeItemPath)
	subrouter = router.Path(p).Subrouter()
	subrouter.Methods(http.MethodGet).HandlerFunc(h.GetPlaneByID)
	subrouter.Methods(http.MethodPut).HandlerFunc(h.CreateOrUpdatePlane)
	subrouter.Methods(http.MethodDelete).HandlerFunc(h.DeletePlaneByID)

	p = fmt.Sprintf("%s%s", baseURL, resourceGroupCollectionPath)
	router.Path(p).Methods("GET").HandlerFunc(h.ListResourceGroups)
	subrouter.Methods(http.MethodGet).HandlerFunc(h.ListResourceGroups)
	p = fmt.Sprintf("%s%s", baseURL, resourceGroupItemPath)
	subrouter = router.Path(p).Subrouter()
	subrouter.Methods(http.MethodGet).HandlerFunc(h.GetResourceGroup)
	subrouter.Methods(http.MethodPut).HandlerFunc(h.CreateResourceGroup)
	subrouter.Methods(http.MethodDelete).HandlerFunc(h.DeleteResourceGroup)

	// Proxy request should take the least priority in routing and should therefore be last
	p = fmt.Sprintf("%s%s", baseURL, planeItemPath)
	router.PathPrefix(p).HandlerFunc(h.ProxyPlaneRequest)

	router.NotFoundHandler = http.HandlerFunc(h.DefaultHandler)
}
