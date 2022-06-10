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
	manager "github.com/project-radius/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/project-radius/radius/pkg/armrpc/authentication"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/handler"
	qprovider "github.com/project-radius/radius/pkg/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
)

const (
	statusResourceType = handler.ProviderNamespaceName + "/operationstatuses"
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
	return handler.ProviderNamespaceName
}

func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	sp := dataprovider.NewStorageProvider(s.Options.Config.StorageProvider)
	qp := qprovider.New(s.Options.Config.QueueProvider)

	opSC, err := sp.GetStorageClient(ctx, statusResourceType)
	if err != nil {
		return err
	}
	qcli, err := qp.GetClient(ctx)
	if err != nil {
		return err
	}
	sm := manager.New(opSC, qcli, handler.ProviderNamespaceName, s.Options.Config.Env.RoleLocation)

	ctx = logr.NewContext(ctx, logger)
	ctx = hostoptions.WithContext(ctx, s.Options.Config)

	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)

	// initialize the manager for ARM client cert validation
	var acm *authentication.ArmCertManager
	if s.Options.Config.Server.EnableArmAuth {
		acm = authentication.NewArmCertManager(s.Options.Config.Server.ArmMetadataEndpoint, logger)
		err := acm.Start(ctx)
		if err != nil {
			logger.Error(err, "Error creating arm cert manager")
			return err
		}
	}
	server, err := server.New(ctx, server.Options{
		Address:  address,
		PathBase: s.Options.Config.Server.PathBase,
		// set the arm cert manager for managing client certificate
		ArmCertMgr:    acm,
		EnableArmAuth: s.Options.Config.Server.EnableArmAuth, // when enabled the client cert validation will be done
		Configure: func(router *mux.Router) error {
			err := handler.AddRoutes(ctx, sp, sm, router, s.Options.Config.Server.PathBase, !hostoptions.IsSelfHosted())
			if err != nil {
				return err
			}

			return nil
		}},
	)
	if err != nil {
		return err
	}

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
