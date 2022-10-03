// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/armrpc/authentication"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/telemetry/metrics"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/pkg/version"
)

const (
	versionEndpoint = "/version"
	healthzEndpoint = "/healthz"
	versionAPIName  = "versionAPI"
	healthzAPIName  = "heathzAPI"
)

type Options struct {
	ProviderNamespace string
	Location          string
	Address           string
	PathBase          string
	EnableArmAuth     bool
	Configure         func(*mux.Router) error
	ArmCertMgr        *authentication.ArmCertManager
}

// New creates a frontend server that can listen on the provided address and serve requests.
func New(ctx context.Context, options Options) (*http.Server, error) {
	r := mux.NewRouter()
	if options.Configure != nil {
		err := options.Configure(r)
		if err != nil {
			return nil, err
		}
	}

	r.NotFoundHandler = validator.APINotFoundHandler()
	r.MethodNotAllowedHandler = validator.APIMethodNotAllowedHandler()

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues)
	// add the arm cert validation if EnableAuth is true
	if options.EnableArmAuth {
		r.Use(authentication.ClientCertValidator(options.ArmCertMgr))
	}
	r.Use(servicecontext.ARMRequestCtx(options.PathBase, options.Location))

	r.Path(versionEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(versionAPIName)
	r.Path(healthzEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(healthzAPIName)
	// setup metrics object
	httpMetrics, err := metrics.NewHTTPMetrics(options.ProviderNamespace)
	if err != nil {
		return nil, err
	}
	r.Use(middleware.MetricsRecorder(httpMetrics))

	server := &http.Server{
		Addr:    options.Address,
		Handler: middleware.LowercaseURLPath(r),
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server, nil
}
