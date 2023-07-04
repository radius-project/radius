/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/ucp/datamodel"
	"github.com/project-radius/radius/pkg/ucp/datamodel/converter"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	aws_frontend "github.com/project-radius/radius/pkg/ucp/frontend/aws"
	azure_frontend "github.com/project-radius/radius/pkg/ucp/frontend/azure"
	"github.com/project-radius/radius/pkg/ucp/frontend/modules"
	radius_frontend "github.com/project-radius/radius/pkg/ucp/frontend/radius"
	"github.com/project-radius/radius/pkg/ucp/frontend/versions"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	queueprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/rest"
	secretprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"
	"github.com/project-radius/radius/pkg/validator"
	"github.com/project-radius/radius/swagger"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/global"
)

const (
	DefaultPlanesConfig = "DEFAULT_PLANES_CONFIG"
)

type ServiceOptions struct {
	// Config is the bootstrap configuration loaded from config file.
	Config *hostoptions.UCPConfig

	ProviderName            string
	Address                 string
	PathBase                string
	Configure               func(chi.Router)
	TLSCertDir              string
	DefaultPlanesConfigFile string
	StorageProviderOptions  dataprovider.StorageProviderOptions
	SecretProviderOptions   secretprovider.SecretProviderOptions
	QueueProviderOptions    queueprovider.QueueProviderOptions
	InitialPlanes           []rest.Plane
	Identity                hostoptions.Identity
	UCPConnection           sdk.Connection
	Location                string

	// Modules is a list of modules that will be registered with the router.
	Modules []modules.Initializer
}

// Service implements the hosting.Service interface for the UCP frontend API.
type Service struct {
	options         ServiceOptions
	storageProvider dataprovider.DataStorageProvider
	queueProvider   *queueprovider.QueueProvider
	secretProvider  *secretprovider.SecretProvider
}

// DefaultModules returns a list of default modules that will be registered with the router.
func DefaultModules(options modules.Options) []modules.Initializer {
	return []modules.Initializer{
		aws_frontend.NewModule(options),
		azure_frontend.NewModule(options),
		radius_frontend.NewModule(options, "radius"),
		radius_frontend.NewModule(options, "deployments"),
	}
}

var _ hosting.Service = (*Service)(nil)

// NewService creates a server to serve UCP API requests.
func NewService(options ServiceOptions) *Service {
	return &Service{
		options: options,
	}
}

// Name gets this service name.
func (s *Service) Name() string {
	return "api"
}

// Initialize sets up the router, storage provider, secret provider, status manager, AWS config, AWS clients,
// registers the routes, configures the default planes, and sets up the http server with the appropriate middleware. It
// returns an http server and an error if one occurs.
func (s *Service) Initialize(ctx context.Context) (*http.Server, error) {
	var err error
	r := chi.NewRouter()

	s.storageProvider = dataprovider.NewStorageProvider(s.options.StorageProviderOptions)
	s.queueProvider = queueprovider.New(s.options.ProviderName, s.options.QueueProviderOptions)
	s.secretProvider = secretprovider.NewSecretProvider(s.options.SecretProviderOptions)

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, []string{s.options.PathBase}, "")
	if err != nil {
		return nil, err
	}

	moduleOptions := modules.Options{
		Address:        s.options.Address,
		PathBase:       s.options.PathBase,
		Config:         s.options.Config,
		Location:       s.options.Location,
		DataProvider:   s.storageProvider,
		QueueProvider:  s.queueProvider,
		SecretProvider: s.secretProvider,
		SpecLoader:     specLoader,
		UCPConnection:  s.options.UCPConnection,
	}

	modules := DefaultModules(moduleOptions)
	err = Register(ctx, r, modules, moduleOptions)
	if err != nil {
		return nil, err
	}

	if s.options.Configure != nil {
		s.options.Configure(r)
	}

	err = s.configureDefaultPlanes(ctx)
	if err != nil {
		return nil, err
	}

	app := http.Handler(r)
	app = servicecontext.ARMRequestCtx(s.options.PathBase, "global")(app)
	app = middleware.WithLogger("ucp")(app)

	app = otelhttp.NewHandler(
		middleware.NormalizePath(app),
		"ucp",
		otelhttp.WithMeterProvider(global.MeterProvider()),
		otelhttp.WithTracerProvider(otel.GetTracerProvider()))

	// TODO: This is the workaround to fix the high cardinality of otelhttp.
	// Remove this once otelhttp middleware is fixed - https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3765
	app = middleware.RemoveRemoteAddr(app)

	server := &http.Server{
		Addr: s.options.Address,
		// Need to be able to respond to requests with planes and resourcegroups segments with any casing e.g.: /Planes, /resourceGroups
		// AWS SDK is case sensitive. Therefore, cannot use lowercase middleware. Therefore, introducing a new middleware that translates
		// the path for only these segments and preserves the case for the other parts of the path.
		// TODO: Once https://github.com/project-radius/radius/issues/3582 is fixed, we could use the lowercase middleware
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}
	return server, nil
}

// configureDefaultPlanes reads the configuration file specified by the env var to configure default planes into UCP
func (s *Service) configureDefaultPlanes(ctx context.Context) error {
	db, err := s.storageProvider.GetStorageClient(ctx, "ucp")
	if err != nil {
		return err
	}

	for _, plane := range s.options.InitialPlanes {
		body, err := json.Marshal(plane)
		if err != nil {
			return err
		}

		opts := armrpc_controller.Options{
			StorageClient: db,
		}
		planesCtrl, err := defaultoperation.NewDefaultSyncPut(opts,
			armrpc_controller.ResourceOptions[datamodel.Plane]{
				RequestConverter:  converter.PlaneDataModelFromVersioned,
				ResponseConverter: converter.PlaneDataModelToVersioned,
			},
		)
		if err != nil {
			return err
		}

		// Using the latest API version to make a request to configure the default planes
		url := fmt.Sprintf("%s?api-version=%s", plane.ID, versions.DefaultAPIVersion)
		request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
		if err != nil {
			return err
		}
		request.Header.Add("Content-Type", "application/json")

		// Wrap the request in an ARM RPC context because this call will bypass the middleware
		// that normally does this for us.
		rpcContext, err := v1.FromARMRequest(request, s.options.PathBase, s.options.Location)
		if err != nil {
			return err
		}
		wrappedCtx := v1.WithARMRequestContext(ctx, rpcContext)

		_, err = planesCtrl.Run(wrappedCtx, nil, request)
		if err != nil {
			return err
		}
	}
	return nil
}

// Run sets up a server to listen on a given address, and shuts it down when the context is done. It returns an
// error if the server fails to start or stops unexpectedly.
func (s *Service) Run(ctx context.Context) error {
	logger := ucplog.FromContextOrDiscard(ctx)
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
