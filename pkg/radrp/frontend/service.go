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
	"github.com/project-radius/radius/pkg/azure/armauth"
	"github.com/project-radius/radius/pkg/model"
	"github.com/project-radius/radius/pkg/radrp/backend/deployment"
	"github.com/project-radius/radius/pkg/radrp/db"
	"github.com/project-radius/radius/pkg/radrp/frontend/handler"
	"github.com/project-radius/radius/pkg/radrp/frontend/resourceprovider"
	"github.com/project-radius/radius/pkg/radrp/frontend/server"
	"github.com/project-radius/radius/pkg/renderers"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	controller_runtime "sigs.k8s.io/controller-runtime/pkg/client"
	gatewayv1alpha1 "sigs.k8s.io/gateway-api/apis/v1alpha1"
	csidriver "sigs.k8s.io/secrets-store-csi-driver/apis/v1alpha1"
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
	logger := logr.FromContextOrDiscard(ctx)

	scheme := clientgoscheme.Scheme
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))
	utilruntime.Must(csidriver.AddToScheme(scheme))

	k8s, err := controller_runtime.New(s.Options.K8sConfig, controller_runtime.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	dbclient, err := s.Options.DBClientFactory(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	var appmodel model.ApplicationModel
	var secretClient renderers.SecretValueClient

	var arm *armauth.ArmConfig
	if s.Options.Arm != nil {
		// Azure credentials have been provided
		arm = s.Options.Arm
		secretClient = renderers.NewSecretValueClient(*s.Options.Arm)
	}
	appmodel, err = model.NewApplicationModel(arm, k8s)
	if err != nil {
		return fmt.Errorf("failed to initialize application model: %w", err)
	}

	urlScheme := "http"
	if s.Options.TLSCertDir != "" {
		urlScheme = "https"
	}
	db := db.NewRadrpDB(dbclient)
	rp := resourceprovider.NewResourceProvider(db, deployment.NewDeploymentProcessor(appmodel, db, &s.Options.HealthChannels, secretClient, k8s), nil, urlScheme, s.Options.BasePath)

	ctx = logr.NewContext(ctx, logger)
	server := server.NewServer(ctx, server.ServerOptions{
		Address:      s.Options.Address,
		Authenticate: s.Options.Authenticate,
		Configure: func(router *mux.Router) {
			handler.AddRoutes(rp, router, handler.DefaultValidatorFactory, s.Options.BasePath)
		},
	})

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = server.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", s.Options.Address))
	if s.Options.TLSCertDir == "" {
		err = server.ListenAndServe()
	} else {
		err = server.ListenAndServeTLS(s.Options.TLSCertDir+"/tls.crt", s.Options.TLSCertDir+"/tls.key")
	}

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
