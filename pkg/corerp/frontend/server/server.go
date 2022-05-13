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
	"github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/corerp/middleware"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/version"
)

const (
	versionEndpoint = "/version"
	healthzEndpoint = "/healthz"
	versionAPIName  = "versionAPI"
	healthzAPIName  = "heathzAPI"
)

type ServerOptions struct {
	Address       string
	PathBase      string
	EnableArmAuth bool
	Configure     func(*mux.Router) error
	ArmCertMgr    *authentication.ArmCertManager
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions) (*http.Server, error) {
	r := mux.NewRouter()
	if options.Configure != nil {
		err := options.Configure(r)
		if err != nil {
			return nil, err
		}
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues)
	// add the arm cert validation if EnableAuth is true
	if options.EnableArmAuth {
		r.Use(middleware.ClientCertValidator(options.ArmCertMgr))
	}
	r.Use(middleware.ARMRequestCtx(options.PathBase))

	r.Path(versionEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(versionAPIName)
	r.Path(healthzEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(healthzAPIName)
	r.Use(provider.NewPrometheusMetricsProvider().MetricsMiddleware)

	server := &http.Server{
		Addr:    options.Address,
		Handler: middleware.LowercaseURLPath(r),
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server, nil
}
