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
	"github.com/project-radius/radius/pkg/version"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
)

type ServerOptions struct {
	Address  string
	PathBase string
	// TODO: implement client cert based authentication for arm
	EnableAuth bool
	Configure  func(*mux.Router)
}

// NewServer will create a server that can listen on the provided address and serve requests.
func NewServer(ctx context.Context, options ServerOptions) *http.Server {
	r := mux.NewRouter()
	if options.Configure != nil {
		options.Configure(r)
	}

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues)
	r.Use(middleware.ARMRequestCtx(options.PathBase))
	r.Path("/version").Methods(http.MethodGet).HandlerFunc(reportVersion)
	r.Path("/healthz").Methods(http.MethodGet).HandlerFunc(reportVersion)

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
	meter := global.GetMeterProvider().Meter("radius-rp")
	counter := metric.Must(meter).NewFloat64Counter("healthzRequests",
		metric.WithDescription("healthz metrics"))
	if err != nil {
		w.WriteHeader(500)
		counter.Add(ctx, 1, attribute.String("healthzFailed", "404"))
		return
	}

	w.Header().Add("Content-Type", "application/json")
	_, _ = w.Write(b)
	counter.Add(ctx, 1, attribute.String("healthzSuccess", "200"))
}
