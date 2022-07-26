// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"
)

//TODO: Use variables and construct the path as we add more APIs.
const (
	planeCollectionPath   = "/planes"
	planeItemPath         = "/planes/{PlaneType}/{PlaneID}"
	planeCollectionByType = "/planes/{PlaneType}"
)

var resourceGroupCollectionPath = fmt.Sprintf("%s/%s", planeItemPath, "resource{[gG]}roups")
var resourceGroupItemPath = fmt.Sprintf("%s/%s", resourceGroupCollectionPath, "{ResourceGroup}")

// Register registers the routes for UCP
func Register(ctx context.Context, router *mux.Router, client store.StorageClient, ucp ucphandler.UCPHandler) error {
	baseURL := ucp.Options.BasePath

	h := Handler{
		db:  client,
		ucp: ucp,
	}

	// If we're in Kubernetes we have some required routes to implement.
	if baseURL != "" {
		// NOTE: the Kubernetes API Server does not include the gvr (base path) in
		// the URL for swagger routes.
		router.Path("/openapi/v2").Methods("GET").HandlerFunc(h.GetOpenAPIv2Doc)

		router.Path(baseURL).Methods("GET").HandlerFunc(h.GetDiscoveryDoc)
	}
	logger := ucplog.GetLogger(ctx)
	logger.Info("UCP base path", baseURL)

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, baseURL+planeCollectionPath)
	if err != nil {
		return err
	}

	subrouter := router.PathPrefix(baseURL + planeCollectionPath).Subrouter()
	subrouter.Use(validator.APIValidatorUCP(specLoader))

	// TODO: Handle trailing slashes for matching routes
	// https://github.com/project-radius/radius/issues/2303
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
	subrouter = router.Path(p).Subrouter()
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

	return nil
}
