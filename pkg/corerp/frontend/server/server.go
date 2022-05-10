// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/corerp/middleware"
	mp "github.com/project-radius/radius/pkg/telemetry/metrics"
	"github.com/project-radius/radius/pkg/version"
)

const (
	versionEndpoint = "/version"
	healthzEndpoint = "/healthz"
	versionAPIName  = "versionAPI"
	healthzAPIName  = "heathzAPI"
)

type ServerOptions struct {
	Address    string
	PathBase   string
	EnableAuth bool
	Configure  func(*mux.Router)
	ArmCertMgr *authentication.ArmCertManager
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions, metricsProviderConfig mp.MetricsOptions) *http.Server {
	r := mux.NewRouter()
	if options.Configure != nil {
		options.Configure(r)
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues)
	// add the arm cert validation if EnableAuth is true
	if options.EnableAuth {
		r.Use(middleware.ClientCertValidator(options.ArmCertMgr))
	}
	r.Use(middleware.ARMRequestCtx(options.PathBase))

	r.Path(versionEndpoint).Methods(http.MethodGet).HandlerFunc(reportVersion).Name(versionAPIName)
	r.Path(healthzEndpoint).Methods(http.MethodGet).HandlerFunc(reportVersion).Name(healthzAPIName)

	//setup metrics handler
	metricsProvider, _ := mp.NewPrometheusMetricsClient()
	promExporter := metricsProvider.GetExporter()
	r.Use(middleware.MetricsInterceptor)
	r.Path(metricsProviderConfig.MetricsOptions.Endpoint).HandlerFunc(promExporter.ServeHTTP)

	server := &http.Server{
		Addr:    options.Address,
		Handler: middleware.LowercaseURLPath(r),
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server
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
