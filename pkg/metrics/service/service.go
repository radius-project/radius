/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metricsservice

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/project-radius/radius/pkg/metrics"
	"github.com/project-radius/radius/pkg/metrics/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
)

type Service struct {
	Options HostOptions
}

// # Function Explanation
//
// NewService creates a new Service instance with the given HostOptions.
func NewService(options HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

// # Function Explanation
//
// Name returns the name of the Service instance.
func (s *Service) Name() string {
	return "Metrics Collector"
}

// # Function Explanation
//
// Run sets up a Prometheus exporter, initializes metrics, creates an HTTP server and handles shutdown based on the
// context, returning an error if one occurs.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)

	pme, err := provider.NewPrometheusExporter(s.Options.Config)
	if err != nil {
		return err
	}

	err = runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second))
	if err != nil {
		logger.Error(err, "failed to start runtime metrics")
	}

	err = metrics.InitMetrics()
	if err != nil {
		logger.Error(err, "failed to initialize metrics")
	}

	mux := http.NewServeMux()
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

	logger.Info("Server stopped...")
	return nil
}
