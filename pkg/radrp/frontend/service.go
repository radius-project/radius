// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package frontend

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/radius/pkg/model/azure"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/deployment"
	"github.com/Azure/radius/pkg/radrp/frontend/handlerv2"
	"github.com/Azure/radius/pkg/radrp/frontend/handlerv3"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv2"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceproviderv3"
	"github.com/Azure/radius/pkg/radrp/frontend/server"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
)

type Service struct {
	Options ServiceOptions
}

func NewService(options ServiceOptions) *Service {
	return &Service{
		Options: options,
	}
}

func (s *Service) Name() string {
	return "frontend"
}

func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContext(ctx)

	k8s, err := controller_runtime.New(s.Options.K8sConfig, controller_runtime.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dbclient, err := s.Options.DBClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	appmodel := azure.NewAzureModel(*s.Options.Arm, k8s)

	db := db.NewRadrpDB(dbclient)
	deploy := deployment.NewDeploymentProcessor(appmodel, &s.Options.HealthChannels)
	rp2 := resourceproviderv2.NewResourceProvider(db, deploy)
	rp3 := resourceproviderv3.NewResourceProvider()

	ctx = logr.NewContext(ctx, logger)
	server := server.NewServer(ctx, server.ServerOptions{
		Address:      s.Options.Address,
		Authenticate: s.Options.Authenticate,
		Configure: func(router *mux.Router) {
			handlerv2.AddRoutes(rp2, router)
			handlerv3.AddRoutes(rp3, router)
		},
	})

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", s.Options.Address))
	err = server.ListenAndServe()
	if err != http.ErrServerClosed {
		// We expect this, safe to ignore.
		logger.Info("Server stopped...")
		return nil
	} else if err != nil {
		return err
	}

	logger.Info("Server stopped...")
	return nil
}
