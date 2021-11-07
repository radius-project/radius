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
	"github.com/Azure/radius/pkg/radrp/backend/deployment"
	"github.com/Azure/radius/pkg/radrp/db"
	"github.com/Azure/radius/pkg/radrp/frontend/handler"
	"github.com/Azure/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/Azure/radius/pkg/radrp/frontend/server"
	"github.com/Azure/radius/pkg/renderers"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
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

	scheme := clientgoscheme.Scheme
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))

	k8s, err := controller_runtime.New(s.Options.K8sConfig, controller_runtime.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dbclient, err := s.Options.DBClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	appmodel := azure.NewAzureModel(*s.Options.Arm, k8s)

	secretClient := renderers.NewSecretValueClient(s.Options.Arm.Auth)

	db := db.NewRadrpDB(dbclient)
	rp := resourceprovider.NewResourceProvider(db, deployment.NewDeploymentProcessor(appmodel, db, &s.Options.HealthChannels, secretClient, k8s), nil)

	ctx = logr.NewContext(ctx, logger)
	server := server.NewServer(ctx, server.ServerOptions{
		Address:      s.Options.Address,
		Authenticate: s.Options.Authenticate,
		Configure: func(router *mux.Router) {
			handler.AddRoutes(rp, router, handler.DefaultValidatorFactory)
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
