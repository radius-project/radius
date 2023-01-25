// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package metricsservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/telemetry/metrics/service/hostoptions"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Service struct {
	Options hostoptions.HostOptions
	Handler http.Handler
}

// NewService of metrics package returns a new Service with the configs needed
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

// Name method of metrics package returns the name of the metrics service
func (s *Service) Name() string {
	return "Metrics Collector"
}

// Run method of metrics package creates a new server for exposing an endpoint to collect metrics from
func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	// pme, err := provider.GetPrometheusExporter()
	// if err != nil {
	// 	logger.Error(err, "Failed to configure prometheus metrics client")
	// 	panic(err)
	// }

	mux := http.NewServeMux()
	handler := otelhttp.NewHandler(mux, "Radius")

	mux.HandleFunc(s.Options.Config.Prometheus.Path, handler.ServeHTTP)
	metricsPort := strconv.Itoa(s.Options.Config.Prometheus.Port)
	server := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: mux,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("Metrics Server listening on localhost port: '%s'...", metricsPort))
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		// We expect this, safe to ignore.
		logger.Info("Server stopped...")
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Server stopped...")
	return nil
}
