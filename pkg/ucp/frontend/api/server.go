// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	DefaultPlanesConfig = "DEFAULT_PLANES_CONFIG"
)

type ServiceOptions struct {
	Address                 string
	ClientConfigSource      *hosting.AsyncValue
	Configure               func(*mux.Router)
	DBClient                store.StorageClient
	TLSCertDir              string
	DefaultPlanesConfigFile string
	UCPConfigFile           string
	BasePath                string
	StorageProviderOptions  dataprovider.StorageProviderOptions
	InitialPlanes           []rest.Plane
}

type Service struct {
	options ServiceOptions
}

var _ hosting.Service = (*Service)(nil)

// NewService will create a server that can listen on the provided address and serve requests.
func NewService(options ServiceOptions) *Service {
	return &Service{
		options: options,
	}
}

func (s *Service) Name() string {
	return "api"
}

func (s *Service) Initialize(ctx context.Context) (*http.Server, error) {
	r := mux.NewRouter()

	if s.options.DBClient == nil {
		dbClient, err := s.InitializeStorageClient(ctx)
		if err != nil {
			return nil, err
		}
		s.options.DBClient = dbClient
	}

	ctrlOpts := ctrl.Options{
		BasePath: s.options.BasePath,
		DB:       s.options.DBClient,
		Address:  s.options.Address,
	}

	err := Register(ctx, r, ctrlOpts)
	if err != nil {
		return nil, err
	}

	if s.options.Configure != nil {
		s.options.Configure(r)
	}

	err = s.ConfigureDefaultPlanes(ctx, s.options.DBClient, s.options.InitialPlanes)
	if err != nil {
		return nil, err
	}

	app := http.Handler(r)
	app = middleware.UseLogValues(app, s.options.BasePath)

	server := &http.Server{
		Addr:    s.options.Address,
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}
	return server, nil
}

func (s *Service) InitializeStorageClient(ctx context.Context) (store.StorageClient, error) {
	var storageClient store.StorageClient

	if s.options.StorageProviderOptions.Provider == dataprovider.TypeETCD {
		s.options.StorageProviderOptions.ETCD.Client = s.options.ClientConfigSource
	}

	storageProvider := dataprovider.NewStorageProvider(s.options.StorageProviderOptions)
	storageClient, err := storageProvider.GetStorageClient(ctx, string(s.options.StorageProviderOptions.Provider))

	if err != nil {
		return nil, err
	}

	return storageClient, nil
}

// ConfigureDefaultPlanes reads the configuration file specified by the env var to configure default planes into UCP
func (s *Service) ConfigureDefaultPlanes(ctx context.Context, dbClient store.StorageClient, planes []rest.Plane) error {

	for _, plane := range planes {
		body, err := json.Marshal(plane)
		if err != nil {
			return err
		}

		planesCtrl, err := planes_ctrl.NewCreateOrUpdatePlane(controller.Options{
			DB: dbClient,
		})
		if err != nil {
			return err
		}

		request, err := http.NewRequest(http.MethodPut, plane.ID, bytes.NewBuffer(body))
		if err != nil {
			return err
		}

		_, err = planesCtrl.Run(ctx, nil, request)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	service, err := s.Initialize(ctx)
	if err != nil {
		return err
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = service.Shutdown(ctx)
	}()

	logger.Info(fmt.Sprintf("listening on: '%s'...", s.options.Address))
	if s.options.TLSCertDir == "" {
		err = service.ListenAndServe()
	} else {
		err = service.ListenAndServeTLS(s.options.TLSCertDir+"/tls.crt", s.options.TLSCertDir+"/tls.key")
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
