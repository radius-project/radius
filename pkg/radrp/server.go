// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package radrp

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/Azure/radius/pkg/radrp/certs"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/resources"
	"github.com/Azure/radius/pkg/version"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ServerOptions is
type ServerOptions struct {
	Address      string
	Authenticate bool
	Deploy       deployment.DeploymentProcessor
	K8s          client.Client
	DB           db.RadrpDB
	Logger       logr.Logger
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(options ServerOptions) *http.Server {
	r := mux.NewRouter()
	var s *mux.Router

	rp := NewResourceProvider(options.DB, options.Deploy)
	h := &handler{rp}

	r.Path(resources.MakeCollectionURITemplate(resources.ApplicationCollectionType)).Methods("GET").HandlerFunc(h.listApplications)
	s = r.Path(resources.MakeResourceURITemplate(resources.ApplicationResourceType)).Subrouter()
	s.Methods("GET").HandlerFunc(h.getApplication)
	s.Methods("PUT").HandlerFunc(h.updateApplication)
	s.Methods("DELETE").HandlerFunc(h.deleteApplication)

	r.Path(resources.MakeCollectionURITemplate(resources.ComponentCollectionType)).Methods("GET").HandlerFunc(h.listComponents)
	s = r.Path(resources.MakeResourceURITemplate(resources.ComponentResourceType)).Subrouter()
	s.Methods("GET").HandlerFunc(h.getComponent)
	s.Methods("PUT").HandlerFunc(h.updateComponent)
	s.Methods("DELETE").HandlerFunc(h.deleteComponent)

	r.Path(resources.MakeCollectionURITemplate(resources.DeploymentCollectionType)).Methods("GET").HandlerFunc(h.listDeployments)
	s = r.Path(resources.MakeResourceURITemplate(resources.DeploymentResourceType)).Subrouter()
	s.Methods("GET").HandlerFunc(h.getDeployment)
	s.Methods("PUT").HandlerFunc(h.updateDeployment)
	s.Methods("DELETE").HandlerFunc(h.deleteDeployment)

	s = r.Path(resources.MakeResourceURITemplate(resources.DeploymentOperationResourceType)).Subrouter()
	s.Methods("GET").HandlerFunc(h.getDeploymentOperation)

	r.Path(resources.MakeCollectionURITemplate(resources.ScopeCollectionType)).Methods("GET").HandlerFunc(h.listScopes)
	s = r.Path(resources.MakeResourceURITemplate(resources.ScopeResourceType)).Subrouter()
	s.Methods("GET").HandlerFunc(h.getScope)
	s.Methods("PUT").HandlerFunc(h.updateScope)
	s.Methods("DELETE").HandlerFunc(h.deleteScope)

	r.Path("/version").Methods("GET").HandlerFunc(reportVersion)

	app := rewrite(r)

	if options.Authenticate {
		app = authenticateCert(app)
	}

	return &http.Server{
		Addr:    options.Address,
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			// return context.WithValue(context.Background(), radlogger.ContextLoggerField, options.Logger)
			return logr.NewContext(context.Background(), options.Logger)
		},
	}
}

func reportVersion(w http.ResponseWriter, req *http.Request) {
	info := version.NewVersionInfo()

	b, err := json.MarshalIndent(&info, "", "  ")
	if err != nil {
		w.WriteHeader(500)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
}

// A custom resource provider typically uses a single HTTP endpoint with the original resource ID
// stuffed in the X-MS-CustomProviders-RequestPath header value.
//
// see: https://docs.microsoft.com/en-us/azure/azure-resource-manager/custom-providers/proxy-resource-endpoint-reference
//
// This middleware allows us to use the traditional resource provider URL space and use a router
// to parse URLs.
func rewrite(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {

		header := r.Header.Get("X-MS-CustomProviders-RequestPath")
		if header != "" {
			log.Printf("Rewriting URL Path to: '%s'", header)
			r.URL.Path = header
			r.URL.RawPath = header
		}

		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func authenticateCert(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/subscriptions/") {
			log.Printf("request is not for a sensitive URL - allowing")
			h.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("X-ARR-ClientCert")
		if header == "" {
			log.Printf("X-ARR-ClientCert as not present")
			w.WriteHeader(401)
			return
		}

		err := certs.Validate(header)
		if err != nil {
			log.Printf("Failed to validate client-cert: %s", err)
			w.WriteHeader(401)
			return
		}

		log.Printf("Client-cert is valid")
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
