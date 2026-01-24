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
	"fmt"

	"github.com/go-chi/chi/v5"

	"github.com/radius-project/radius/pkg/armrpc/builder"
	apictrl "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/server"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
)

// RouteConfigurer is a function that configures additional routes on the router.
type RouteConfigurer func(ctx context.Context, r chi.Router, options hostoptions.HostOptions) error

// APIService is the restful API server for Radius Resource Provider.
type APIService struct {
	server.Service

	handlerBuilder   []builder.Builder
	routeConfigurers []RouteConfigurer
}

// NewAPIService creates a new instance of APIService.
func NewAPIService(options hostoptions.HostOptions, builder []builder.Builder) *APIService {
	return &APIService{
		Service: server.Service{
			ProviderName: "radius",
			Options:      options,
		},
		handlerBuilder: builder,
	}
}

// NewAPIServiceWithRoutes creates a new instance of APIService with additional route configurers.
func NewAPIServiceWithRoutes(options hostoptions.HostOptions, builder []builder.Builder, routes ...RouteConfigurer) *APIService {
	return &APIService{
		Service: server.Service{
			ProviderName: "radius",
			Options:      options,
		},
		handlerBuilder:   builder,
		routeConfigurers: routes,
	}
}

// Name returns the name of the service.
func (s *APIService) Name() string {
	return "radiusapi"
}

// Run starts the service.
func (s *APIService) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	databaseClient, err := s.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)
	return s.Start(ctx, server.Options{
		Location: s.Options.Config.Env.RoleLocation,
		Address:  address,
		PathBase: s.Options.Config.Server.PathBase,
		Configure: func(r chi.Router) error {
			baseOpts := apictrl.Options{
				Address:        address,
				PathBase:       s.Options.Config.Server.PathBase,
				DatabaseClient: databaseClient,
				Arm:            s.Options.Arm, // This is a temporary fix to avoid ARM initialization in the test environment.
				KubeClient:     s.KubeClient,
				StatusManager:  s.OperationStatusManager,
			}
			for _, b := range s.handlerBuilder {
				opts := baseOpts

				validator, err := builder.NewOpenAPIValidator(ctx, opts.PathBase, b.Namespace())
				if err != nil {
					panic(err)
				}

				if err := b.ApplyAPIHandlers(ctx, r, opts, validator); err != nil {
					panic(err)
				}
			}

			// Apply additional route configurers (e.g., terraform installer)
			for _, configurer := range s.routeConfigurers {
				if err := configurer(ctx, r, s.Options); err != nil {
					return err
				}
			}

			return nil
		},
		// set the arm cert manager for managing client certificate
		ArmCertMgr:    s.ARMCertManager,
		EnableArmAuth: s.Options.Config.Server.EnableArmAuth, // when enabled the client cert validation will be done
	})
}
