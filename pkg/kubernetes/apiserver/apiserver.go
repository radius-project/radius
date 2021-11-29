// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package apiserver

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/radius/pkg/model"
	"github.com/Azure/radius/pkg/radrp/frontend/server"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type APIServerExtension struct {
	log     logr.Logger
	options APIServerExtensionOptions
}

type APIServerExtensionOptions struct {
	KubeConfig *rest.Config
	Scheme     *apiruntime.Scheme
	AppModel   model.ApplicationModel
	TLSCertDir string
	Port       int
	Start      <-chan struct{}
}

func NewAPIServerExtension(log logr.Logger, options APIServerExtensionOptions) *APIServerExtension {
	return &APIServerExtension{
		log:     log,
		options: options,
	}
}

func (api *APIServerExtension) Name() string {
	return "Radius API Server extension"
}

func (api *APIServerExtension) Run(ctx context.Context) error {
	logger := api.log

	logger.Info("API Server Extension waiting for API Server...")
	<-api.options.Start

	c, err := client.New(api.options.KubeConfig, client.Options{Scheme: api.options.Scheme})
	if err != nil {
		return err
	}
	// Add group/version as prefix.

	rp := NewResourceProvider(api.options.AppModel, c)
	s := server.NewServer(ctx, server.ServerOptions{
		Address:      fmt.Sprintf(":%v", api.options.Port),
		Authenticate: false,
		Configure: func(r *mux.Router) {
			AddRoutes(rp, r, DefaultValidatorFactory)
		},
	})

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = s.Shutdown(ctx)
	}()

	// http://localhost:8001/apis/api.radius.dev/v1alpha3/subscriptions/123/resourceGroups/testrg/providers/Microsoft.CustomProviders/resourceProviders/radiusv3/Application

	logger.Info(fmt.Sprintf("listening on: '%s'...", fmt.Sprintf(":%v", api.options.Port)))
	err = s.ListenAndServeTLS(api.options.TLSCertDir+"/tls.crt", api.options.TLSCertDir+"/tls.key")

	fmt.Println("failed to listen")
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
