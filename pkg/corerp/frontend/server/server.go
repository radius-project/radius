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
	"github.com/project-radius/radius/pkg/corerp/middleware"
	mp "github.com/project-radius/radius/pkg/telemetry/metricsprovider"
	"github.com/project-radius/radius/pkg/version"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/unit"
)

var (
	promMetricsClient *mp.PrometheusMetricsClient
)

type ServerOptions struct {
	Address  string
	PathBase string
	// TODO: implement client cert based authentication for arm
	EnableAuth bool
	Configure  func(*mux.Router)
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions, metricsProviderConfig mp.MetricsClientProviderOptions) *http.Server {
	r := mux.NewRouter()
	if options.Configure != nil {
		options.Configure(r)
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues)
	r.Use(middleware.ARMRequestCtx(options.PathBase))
	r.Use(middleware.MetricsInterceptor)
	r.Path("/version").Methods(http.MethodGet).HandlerFunc(reportVersion)
	r.Path("/healthz").Methods(http.MethodGet).HandlerFunc(reportVersion)

	//setup metrics handler
	promMetricsClient, _ = mp.NewPrometheusMetricsClient()
	r.Path(metricsProviderConfig.MetricsClientProviderOptions.Endpoint).HandlerFunc(promMetricsClient.Client.ServeHTTP)

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
	ctx := req.Context()

	if err != nil {
		w.WriteHeader(500)
		promMetricsClient.Observe(ctx, 1, "radcorerp_system_liveliness_failed", unit.Dimensionless, attribute.String("status","Failed"))
		return
	}
	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
	promMetricsClient.Observe(ctx, 1, "radcorerp_system_liveliness", unit.Dimensionless, attribute.String("status","Success"))
}
