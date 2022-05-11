package metrics

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/project-radius/radius/pkg/telemetry/metrics/hostoptions"
	"github.com/project-radius/radius/pkg/telemetry/metrics/provider"
)

type Service struct {
	Options hostoptions.HostOptions
}

//NewService of metrics package returns a new Service with the configs needed
func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

//Name method of metrics package returns the name of the metrics service
func (s *Service) Name() string {
	return "Metrics Collector"
}

//Run method of metrics package creates a new server for exposing an endpoint to collect metrics from
func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Starting metrics server")

	metricsProvider, err := provider.NewPrometheusMetricsClient()
	exporter := metricsProvider.GetExporter()
	if err != nil {
		logger.Info("Error")
	}

	http.HandleFunc(s.Options.Config.Prometheus.Endpoint, exporter.ServeHTTP)
	err = http.ListenAndServe(":"+strconv.Itoa(s.Options.Config.Prometheus.Port), nil)

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
