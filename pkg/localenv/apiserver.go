// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package localenv

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/radius/pkg/kubernetes/apiserver"
	"github.com/Azure/radius/pkg/radrp/frontend/handler"
	"github.com/Azure/radius/pkg/radrp/frontend/server"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type APIServerExtension struct {
	log     logr.Logger
	options APIServerExtensionOptions
}

type APIServerExtensionOptions struct {
	KubeConfigPath string
	Scheme         *apiruntime.Scheme
	Start          <-chan struct{}
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

	config, err := GetRESTConfig(api.options.KubeConfigPath)
	if err != nil {
		return err
	}

	c, err := client.New(config, client.Options{Scheme: api.options.Scheme})
	if err != nil {
		return err
	}

	rp := apiserver.NewResourceProvider(c)
	s := server.NewServer(ctx, server.ServerOptions{
		Address:      "localhost:9999",
		Authenticate: false,
		Configure: func(r *mux.Router) {
			apiserver.AddRoutes(rp, r, handler.DefaultValidatorFactory)
		},
	})

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = s.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", "localhost:9999"))
	err = s.ListenAndServe()
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
