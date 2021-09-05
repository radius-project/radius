// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package handlerv2

import (
	"github.com/Azure/radius/pkg/azure/azresources"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/gorilla/mux"
)

func AddRoutes(rp resourceprovider.ResourceProvider, router *mux.Router) {
	h := handler{rp}
	var subrouter *mux.Router

	router.Path(azresources.MakeCollectionURITemplate(resources.ApplicationCollectionType)).Methods("GET").HandlerFunc(h.listApplications)
	subrouter = router.Path(azresources.MakeResourceURITemplate(resources.ApplicationResourceType)).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.getApplication)
	subrouter.Methods("PUT").HandlerFunc(h.updateApplication)
	subrouter.Methods("DELETE").HandlerFunc(h.deleteApplication)

	router.Path(azresources.MakeCollectionURITemplate(resources.ComponentCollectionType)).Methods("GET").HandlerFunc(h.listComponents)
	subrouter = router.Path(azresources.MakeResourceURITemplate(resources.ComponentResourceType)).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.getComponent)
	subrouter.Methods("PUT").HandlerFunc(h.updateComponent)
	subrouter.Methods("DELETE").HandlerFunc(h.deleteComponent)

	router.Path(azresources.MakeCollectionURITemplate(resources.DeploymentCollectionType)).Methods("GET").HandlerFunc(h.listDeployments)
	subrouter = router.Path(azresources.MakeResourceURITemplate(resources.DeploymentResourceType)).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.getDeployment)
	subrouter.Methods("PUT").HandlerFunc(h.updateDeployment)
	subrouter.Methods("DELETE").HandlerFunc(h.deleteDeployment)

	subrouter = router.Path(azresources.MakeResourceURITemplate(resources.DeploymentOperationResourceType)).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.getDeploymentOperation)

	router.Path(azresources.MakeCollectionURITemplate(resources.ScopeCollectionType)).Methods("GET").HandlerFunc(h.listScopes)
	subrouter = router.Path(azresources.MakeResourceURITemplate(resources.ScopeResourceType)).Subrouter()
	subrouter.Methods("GET").HandlerFunc(h.getScope)
	subrouter.Methods("PUT").HandlerFunc(h.updateScope)
	subrouter.Methods("DELETE").HandlerFunc(h.deleteScope)
}
