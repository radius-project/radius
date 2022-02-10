// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/azure/azresources"
	"github.com/project-radius/radius/pkg/radlogger"
	"github.com/project-radius/radius/pkg/radrp/certs"
	"github.com/project-radius/radius/pkg/version"
)

type ServerOptions struct {
	Address      string
	Authenticate bool
	Configure    func(*mux.Router)
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions) *http.Server {
	r := mux.NewRouter()
	if options.Configure != nil {
		options.Configure(r)
	}

	r.Path("/version").Methods("GET").HandlerFunc(reportVersion)

	app := rewrite(r)
	app = appendLogValues(app)

	if options.Authenticate {
		app = authenticateCert(app)
	}

	return &http.Server{
		Addr:    options.Address,
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
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
		logger := radlogger.GetLogger(r.Context())
		header := r.Header.Get("X-MS-CustomProviders-RequestPath")
		if header != "" {
			logger.V(radlogger.Verbose).Info(fmt.Sprintf("Rewriting URL Path to: '%s'", header))
			r.URL.Path = header
			r.URL.RawPath = header
		}
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// Append logging values to the context based on the Resource ID (if present).
func appendLogValues(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		id, err := azresources.Parse(r.URL.Path)
		if err != nil {
			// This just means the request is for an ARM resource. Not an error.
			h.ServeHTTP(w, r)
			return
		}

		values := []interface{}{}
		values = append(values, radlogger.LogFieldResourceID, id.ID)
		values = append(values, radlogger.LogFieldSubscriptionID, id.SubscriptionID)
		values = append(values, radlogger.LogFieldResourceGroup, id.ResourceGroup)
		values = append(values, radlogger.LogFieldResourceType, id.Type())
		values = append(values, radlogger.LogFieldResourceName, id.QualifiedName())

		r = r.WithContext(radlogger.WrapLogContext(r.Context(), values...))
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func authenticateCert(h http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		logger := radlogger.GetLogger(r.Context())
		if !strings.HasPrefix(r.URL.Path, "/subscriptions/") {
			logger.V(radlogger.Verbose).Info("request is not for a sensitive URL - allowing")
			h.ServeHTTP(w, r)
			return
		}

		header := r.Header.Get("X-ARR-ClientCert")
		if header == "" {
			logger.V(radlogger.Verbose).Info("X-ARR-ClientCert as not present")
			w.WriteHeader(401)
			return
		}

		err := certs.Validate(header)
		if err != nil {
			logger.Error(err, "Failed to validate client-cert")
			w.WriteHeader(401)
			return
		}

		logger.V(radlogger.Verbose).Info("Client-cert is valid")
		h.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}
