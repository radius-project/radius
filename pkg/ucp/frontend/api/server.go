// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	"github.com/project-radius/radius/pkg/ucp/frontend/middleware"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler/planes"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
	"github.com/project-radius/radius/pkg/ucp/store/etcdstore"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const (
	DefaultPlanesConfig = "DEFAULT_PLANES_CONFIG"
)

type ServiceOptions struct {
	Address                 string
	ClientConfigSource      *hosting.AsyncValue
	Configure               func(*mux.Router)
	UcpHandler              ucphandler.UCPHandler
	DBClient                store.StorageClient
	TLSCertDir              string
	DefaultPlanesConfigFile string
	BasePath                string
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
		// Initialize the storage client once the storage service has started
		obj, err := s.options.ClientConfigSource.Get(ctx)
		if err != nil {
			return nil, err
		}

		clientconfig := obj.(*etcdclient.Config)
		client, err := etcdclient.New(*clientconfig)
		if err != nil {
			return nil, err
		}
		etcdClient := etcdstore.NewETCDClient(client)
		s.options.DBClient = etcdClient
	}
	Register(r, s.options.DBClient, s.options.UcpHandler)
	if s.options.Configure != nil {
		s.options.Configure(r)
	}

	err := s.ConfigureDefaultPlanes(ctx, s.options.DBClient, s.options.UcpHandler.Planes)
	if err != nil {
		return nil, err
	}

	app := http.Handler(r)
	app = middleware.UseLogValues(app)

	server := &http.Server{
		Addr:    s.options.Address,
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}
	return server, nil
}

// ConfigureDefaultPlanes reads the configuration file specified by the env var to configure default planes into UCP
func (s *Service) ConfigureDefaultPlanes(ctx context.Context, dbClient store.StorageClient, planesUCPHandler planes.PlanesUCPHandler) error {
	if s.options.DefaultPlanesConfigFile == "" {
		// No default planes to configure
		return nil
	}
	// Read the default planes confiuration file and configure the planes
	data, err := ioutil.ReadFile(s.options.DefaultPlanesConfigFile)
	if err != nil {
		return err
	}
	var planes = []rest.Plane{}
	err = json.Unmarshal(data, &planes)
	if err != nil {
		return err
	}

	for _, plane := range planes {
		bytes, err := json.Marshal(plane)
		if err != nil {
			return err
		}

		_, err = planesUCPHandler.CreateOrUpdate(ctx, dbClient, bytes, plane.ID)
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
