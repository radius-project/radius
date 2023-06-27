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

package server

import (
	"context"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/project-radius/radius/pkg/armrpc/authentication"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/pkg/version"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
)

const (
	versionEndpoint = "/version"
	healthzEndpoint = "/healthz"
	versionAPIName  = "versionAPI"
	healthzAPIName  = "heathzAPI"
)

type Options struct {
	ProviderNamespace string
	Location          string
	Address           string
	PathBase          string
	EnableArmAuth     bool
	Configure         func(*mux.Router) error
	ArmCertMgr        *authentication.ArmCertManager
}

// New creates a frontend server that can listen on the provided address and serve requests.
//
// # Function Explanation
// 
//	New creates a new HTTP server with the given context and options. It configures the router with the given options, adds 
//	middleware for error handling and ARM authentication, and sets up the version and healthz endpoints. If an error occurs 
//	during configuration, it is returned.
func New(ctx context.Context, options Options) (*http.Server, error) {
	r := mux.NewRouter()
	if options.Configure != nil {
		err := options.Configure(r)
		if err != nil {
			return nil, err
		}
	}

	r.NotFoundHandler = validator.APINotFoundHandler()
	r.MethodNotAllowedHandler = validator.APIMethodNotAllowedHandler()

	r.Use(middleware.Recoverer)
	r.Use(middleware.AppendLogValues(options.ProviderNamespace))

	// add the arm cert validation if EnableAuth is true
	if options.EnableArmAuth {
		r.Use(authentication.ClientCertValidator(options.ArmCertMgr))
	}
	r.Use(servicecontext.ARMRequestCtx(options.PathBase, options.Location))

	r.Path(versionEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(versionAPIName)
	r.Path(healthzEndpoint).Methods(http.MethodGet).HandlerFunc(version.ReportVersionHandler).Name(healthzAPIName)

	handlerFunc := otelhttp.NewHandler(
		middleware.LowercaseURLPath(r),
		options.ProviderNamespace,
		otelhttp.WithMeterProvider(global.MeterProvider()),
		otelhttp.WithTracerProvider(otel.GetTracerProvider()))

	// TODO: This is the workaround to fix the high cardinality of otelhttp.
	// Remove this once otelhttp middleware is fixed - https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3765
	handlerFunc = middleware.RemoveRemoteAddr(handlerFunc)

	server := &http.Server{
		Addr:    options.Address,
		Handler: handlerFunc,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}

	return server, nil
}
