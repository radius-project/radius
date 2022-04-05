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

	// TODO: Add DB Provider and DeploymentProcessor.

	ctx = logr.NewContext(ctx, logger)
	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)
	server := server.NewServer(ctx, server.ServerOptions{
		Address: address,
		// TODO: implement ARM client certificate auth.
		Configure: func(router *mux.Router) {
			handler.AddRoutes(nil, nil, router, handler.DefaultValidatorFactory, "")
		},
	})

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", address))
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
