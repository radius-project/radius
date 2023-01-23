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
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/telemetry/metrics/service/hostoptions"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
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

// func initMeter() (*sdkmetric.MeterProvider, error) {
// 	fmt.Println("Initializing Meter Provider...")

// 	exporter, err := prometheus.New()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(exporter))
// 	global.SetMeterProvider(mp)

// 	return mp, nil
// }

// Run method of metrics package creates a new server for exposing an endpoint to collect metrics from
func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	// TODO: Check if the metrics are enabled.
	// 1. Create a new global meter provider
	// mp, err := initMeter()
	// if err != nil {
	// 	log.Fatal(err)
	// 	return err
	// }
	// defer func() {
	// 	if err := mp.Shutdown(context.Background()); err != nil {
	// 		log.Printf("Error shutting down meter provider: %v", err)
	// 	}
	// }()

	exporter, err := provider.NewPrometheusExporter()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.Options.Config.Prometheus.Path, exporter.Handler.ServeHTTP)
	metricsPort := strconv.Itoa(s.Options.Config.Prometheus.Port)
	server := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: otelhttp.NewHandler(mux, "metrics-service", otelhttp.WithMeterProvider(exporter.MeterProvider), otelhttp.WithTracerProvider(otel.GetTracerProvider())),
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
