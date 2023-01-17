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
	"time"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	"github.com/project-radius/radius/pkg/telemetry/metrics/service/hostoptions"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "radius_processed_ops_total",
		Help: "The total number of processed events",
	})
)

func recordMetrics() {
	go func() {
		for {
			opsProcessed.Inc()
			time.Sleep(2 * time.Second)
		}
	}()
}

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

	pme, err := provider.GetPrometheusExporter()
	if err != nil {
		logger.Error(err, "Failed to configure prometheus metrics client")
		panic(err)
	}

	recordMetrics()

	mux := http.NewServeMux()
	// TODO: otelhttp.NewHandler...
	mux.HandleFunc(s.Options.Config.Prometheus.Path, pme.Handler.ServeHTTP)
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

	// meter := pme.MeterProvider.Meter("radius")
	// attrs := []attribute.KeyValue{
	// 	attribute.Key("A").String("B"),
	// 	attribute.Key("C").String("D"),
	// }
	// counter, err := meter.SyncFloat64().Counter("foo", instrument.WithDescription("a simple counter"))
	// if err != nil {
	// 	fmt.Println("SyncFloat64 error: ", err)
	// }
	// counter.Add(ctx, 5, attrs...)

	logger.Info("Server stopped...")
	return nil
}
