// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	armAuthenticator "github.com/project-radius/radius/pkg/corerp/authentication"
	"github.com/project-radius/radius/pkg/corerp/dataprovider"
	"github.com/project-radius/radius/pkg/corerp/frontend/handler"
	"github.com/project-radius/radius/pkg/corerp/frontend/server"
	"github.com/project-radius/radius/pkg/corerp/hostoptions"
)

type Service struct {
	Options hostoptions.HostOptions
}

func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		Options: options,
	}
}

func (s *Service) Name() string {
	return "Applications.Core RP frontend"
}

func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	storageProvider := dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)

	ctx = logr.NewContext(ctx, logger)
	ctx = hostoptions.WithContext(ctx, s.Options.Config)

	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)
<<<<<<< HEAD
	server := server.NewServer(ctx,
		server.ServerOptions{
			Address:  address,
			PathBase: s.Options.Config.Server.PathBase,
			// TODO: implement ARM client certificate auth.
			Configure: func(router *mux.Router) {
				if err := handler.AddRoutes(ctx, storageProvider, nil, router, handler.DefaultValidatorFactory, ""); err != nil {
					panic(err)
				}

				// TODO Connector RP will be moved into a separate service, for now using core RP's infra to unblock end to end testing
				// https://github.com/project-radius/core-team/issues/90
				if err := handler.AddConnectorRoutes(ctx, storageProvider, nil, router, handler.DefaultValidatorFactory, ""); err != nil {
					panic(err)
				}
			},
		},
=======

	// initialize the manager for ARM client cert validation
	armCertMgr, err := NewArmCertManager(s.Options.Config.Server.ArmMetadataEndpoint)
	if err != nil || armCertMgr == nil {
		logger.Info("Error creating arm cert manager - ", err)
	}
	server := server.NewServer(ctx, server.ServerOptions{
		Address:  address,
		PathBase: s.Options.Config.Server.PathBase,
		// set the arm cert manager for managing client certificate
		ArmCertMgr: armCertMgr,
		EnableAuth: s.Options.Config.Server.EnableAuth, // when enabled the client cert validation will be done
		Configure: func(router *mux.Router) {
			if err := handler.AddRoutes(ctx, storageProvider, nil, router, handler.DefaultValidatorFactory, ""); err != nil {
				panic(err)
			}
		}},
>>>>>>> 3575b301 (fix review comments)
		s.Options.Config.MetricsProvider,
	)

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", address))
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

// NewArmCertManager creates arm cert manager that fetches/refreshes the arm client cert from metadata endpoint
func NewArmCertManager(armMetaEndpoint string) (*armAuthenticator.ArmCertManager, error) {
	armCertManager := armAuthenticator.NewArmCertManager(armMetaEndpoint)
	_, err := armCertManager.Start(context.Background())
	if err != nil {
		return armCertManager, err
	}
	return armCertManager, nil
}
