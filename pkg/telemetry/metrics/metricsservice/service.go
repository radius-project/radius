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
	"github.com/project-radius/radius/pkg/telemetry/metrics/metricsservice/hostoptions"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"go.opentelemetry.io/otel/exporters/metric/prometheus"
)

type Service struct {
	Options hostoptions.HostOptions
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

	promConfig := prometheus.Config{
		// buckets distribution used for histogram_quantile to calculate p50, p75, p95, p99 values
		DefaultHistogramBoundaries: []float64{1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 5000, 10000, 20000, 50000, 100000},
	}
	exporter, err := provider.NewPrometheusMetricsExporter(promConfig)
	if err != nil {
		logger.Error(err, "Failed to configure prometheus metrics client")
		panic(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.Options.Config.Prometheus.Endpoint, exporter.ServeHTTP)
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
	err = server.ListenAndServe()
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
