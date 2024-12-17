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

package frontend

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/radius-project/radius/pkg/armrpc/servicecontext"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp/ucplog"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

func NewService(options *dynamicrp.Options) *Service {
	return &Service{
		options: options,
	}
}

// Service implements the hosting.Service interface for the UCP frontend API.
type Service struct {
	options *dynamicrp.Options
}

// Name gets this service name.
func (s *Service) Name() string {
	return "dynamic-rp api"
}

// Initialize sets up the router, database provider, secret provider, status manager, AWS config, AWS clients,
// registers the routes, configures the default planes, and sets up the http server with the appropriate middleware. It
// returns an http server and an error if one occurs.
func (s *Service) initialize(ctx context.Context) (*http.Server, error) {
	r := chi.NewRouter()

	err := s.registerRoutes(r)
	if err != nil {
		return nil, fmt.Errorf("failed to register routes: %w", err)
	}

	app := http.Handler(r)

	// Autodetect pathbase
	app = servicecontext.ARMRequestCtx("", s.options.Config.Environment.RoleLocation)(app)
	app = middleware.WithLogger(app)

	app = otelhttp.NewHandler(
		middleware.NormalizePath(app),
		"dynamic-rp",
		otelhttp.WithMeterProvider(otel.GetMeterProvider()),
		otelhttp.WithTracerProvider(otel.GetTracerProvider()))

	// TODO: This is the workaround to fix the high cardinality of otelhttp.
	// Remove this once otelhttp middleware is fixed - https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3765
	app = middleware.RemoveRemoteAddr(app)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.options.Config.Server.Host, s.options.Config.Server.Port),
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server, nil
}

// Run sets up a server to listen on a given address, and shuts it down when the context is done. It returns an
// error if the server fails to start or stops unexpectedly.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
	server, err := s.initialize(ctx)
	if err != nil {
		return err
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", server.Addr))
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
