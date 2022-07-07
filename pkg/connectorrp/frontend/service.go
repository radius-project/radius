// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"fmt"

	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/armrpc/frontend/server"
	"github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/deployment"
	"github.com/project-radius/radius/pkg/connectorrp/frontend/handler"
	"github.com/project-radius/radius/pkg/connectorrp/model"

	ctrl "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
)

type Service struct {
	server.Service
}

func NewService(options hostoptions.HostOptions) *Service {
	return &Service{
		server.Service{
			Options:      options,
			ProviderName: handler.ProviderNamespaceName,
		},
	}
}

func (s *Service) Name() string {
	return handler.ProviderNamespaceName
}

func (s *Service) Run(ctx context.Context) error {
	if err := s.Init(ctx); err != nil {
		return err
	}

	connectorAppModel, err := model.NewApplicationModel(s.Options.Arm, s.KubeClient)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	opts := ctrl.Options{
		DataProvider: s.StorageProvider,
		SecretClient: s.SecretClient,
		KubeClient:   s.KubeClient,
		GetDeploymentProcessor: func() deployment.DeploymentProcessor {
			return deployment.NewDeploymentProcessor(connectorAppModel, s.StorageProvider, s.SecretClient, s.KubeClient)
		},
	}

	address := fmt.Sprintf("%s:%d", s.Options.Config.Server.Host, s.Options.Config.Server.Port)
	err = s.Start(ctx, server.Options{
		Address:  address,
		PathBase: s.Options.Config.Server.PathBase,
		// set the arm cert manager for managing client certificate
		ArmCertMgr:    s.ARMCertManager,
		EnableArmAuth: s.Options.Config.Server.EnableArmAuth, // when enabled the client cert validation will be done
		Configure: func(router *mux.Router) error {
			err := handler.AddRoutes(ctx, router, s.Options.Config.Server.PathBase, !hostoptions.IsSelfHosted(), opts)
			if err != nil {
				return err
			}
			return nil
		}},
	)
	return err
}
