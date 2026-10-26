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
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/radius-project/radius/pkg/armrpc/frontend/controller"
	"github.com/radius-project/radius/pkg/armrpc/frontend/defaultoperation"
	"github.com/radius-project/radius/pkg/armrpc/servicecontext"
	"github.com/radius-project/radius/pkg/authz"
	"github.com/radius-project/radius/pkg/authz/mtls"
	"github.com/radius-project/radius/pkg/components/hosting"
	"github.com/radius-project/radius/pkg/kubeutil"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/authzcheck"
	"github.com/radius-project/radius/pkg/ucp/datamodel"
	"github.com/radius-project/radius/pkg/ucp/datamodel/converter"
	aws_frontend "github.com/radius-project/radius/pkg/ucp/frontend/aws"
	azure_frontend "github.com/radius-project/radius/pkg/ucp/frontend/azure"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	radius_frontend "github.com/radius-project/radius/pkg/ucp/frontend/radius"
	"github.com/radius-project/radius/pkg/ucp/frontend/versions"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/pkg/ucp/ucplog"
	"github.com/radius-project/radius/pkg/ucp/userauth"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"k8s.io/client-go/kubernetes"
)

// Service implements the hosting.Service interface for the UCP frontend API.
type Service struct {
	options           *ucp.Options
	userAuthorization *userauth.Runtime
}

// DefaultModules returns a list of default modules that will be registered with the router.
func DefaultModules(options *ucp.Options) []modules.Initializer {
	return []modules.Initializer{
		aws_frontend.NewModule(options),
		azure_frontend.NewModule(options),
		radius_frontend.NewModule(options),
	}
}

var _ hosting.Service = (*Service)(nil)

// NewService creates a server to serve UCP API requests.
func NewService(options *ucp.Options) *Service {
	return &Service{
		options: options,
	}
}

// Name gets this service name.
func (s *Service) Name() string {
	return "api"
}

// Initialize sets up the router, database provider, secret provider, status manager, AWS config, AWS clients,
// registers the routes, configures the default planes, and sets up the http server with the appropriate middleware. It
// returns an http server and an error if one occurs.
func (s *Service) Initialize(ctx context.Context) (*http.Server, error) {
	r := chi.NewRouter()

	// Allow tests to override the default modules.
	modules := s.options.Modules
	if modules == nil {
		// If unset, use the default modules.
		modules = DefaultModules(s.options)
	}

	err := Register(ctx, r, modules, s.options)
	if err != nil {
		return nil, err
	}

	err = s.configureDefaultPlanes(ctx)
	if err != nil {
		return nil, err
	}

	app := http.Handler(r)
	app = servicecontext.ARMRequestCtx(s.options.Config.Server.PathBase, s.options.Config.Environment.RoleLocation)(app)
	endpoints := &authzcheck.Endpoints{PathBase: s.options.Config.Server.PathBase, DB: s.options.DatabaseProvider.GetClient}
	app = endpoints.Middleware(app)
	if s.options.Config.UserAuthorization.Enabled {
		config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{})
		if err != nil {
			return nil, err
		}
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
		db, err := s.options.DatabaseProvider.GetClient(ctx)
		if err != nil {
			return nil, err
		}
		s.userAuthorization, err = userauth.New(db, client, s.options.Config.UserAuthorization,
			s.options.Config.Server.PathBase, s.options.Config.Authorization.EffectiveMode())
		if err != nil {
			return nil, err
		}
		// An absent anchor remains unavailable until the explicit SAR-guarded
		// initialization request. Never silently bootstrap during a restart.
		if err := s.userAuthorization.Cache.Refresh(ctx); err != nil {
			ucplog.FromContextOrDiscard(ctx).Error(err, "user authorization is not ready; explicit initialization may be required")
		}
		app = s.userAuthorization.Middleware(app)
	}
	authn, err := s.authenticator()
	if err != nil {
		return nil, err
	}
	app = authn.Middleware(app)
	app = middleware.WithLogger(app)

	app = otelhttp.NewHandler(
		middleware.NormalizePath(app),
		"ucp",
		otelhttp.WithMeterProvider(otel.GetMeterProvider()),
		otelhttp.WithTracerProvider(otel.GetTracerProvider()))

	// TODO: This is the workaround to fix the high cardinality of otelhttp.
	// Remove this once otelhttp middleware is fixed - https://github.com/open-telemetry/opentelemetry-go-contrib/issues/3765
	app = middleware.RemoveRemoteAddr(app)
	app = authzcheck.CaptureRemoteAddr(app)

	server := &http.Server{
		Addr: s.options.Config.Server.Address(),
		// Need to be able to respond to requests with planes and resourcegroups segments with any casing e.g.: /Planes, /resourceGroups
		// AWS SDK is case sensitive. Therefore, cannot use lowercase middleware. Therefore, introducing a new middleware that translates
		// the path for only these segments and preserves the case for the other parts of the path.
		// TODO: https://github.com/radius-project/radius/issues/5921
		Handler: app,
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}
	return server, nil
}

// authenticator returns the caller authentication middleware (A6). It is a no-op when
// the authorization mode is off.
func (s *Service) authenticator() (*authzcheck.Authenticator, error) {
	mode := s.options.Config.Authorization.EffectiveMode()
	if mode == authz.ModeOff {
		return &authzcheck.Authenticator{Mode: mode}, nil
	}

	config, err := kubeutil.NewClientConfig(&kubeutil.ConfigOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config for authorization: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &authzcheck.Authenticator{
		Mode:       mode,
		FrontProxy: &authzcheck.KubeFrontProxyVerifier{Client: client},
		Engine:     &authzcheck.KubeEngineResolver{Client: client, Namespace: s.options.Config.Authorization.Namespace},
	}, nil
}

// configureDefaultPlanes reads the configuration file specified by the env var to configure default planes into UCP
func (s *Service) configureDefaultPlanes(ctx context.Context) error {
	for _, plane := range s.options.Config.Initialization.Planes {
		err := s.createPlane(ctx, plane)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) createPlane(ctx context.Context, plane ucp.Plane) error {
	body, err := json.Marshal(plane)
	if err != nil {
		return err
	}

	resourceID, err := resources.ParseScope(plane.ID)
	if err != nil {
		return err
	}

	if len(resourceID.ScopeSegments()) != 1 {
		return fmt.Errorf("invalid plane ID: %s", plane.ID)
	}

	db, err := s.options.DatabaseProvider.GetClient(ctx)
	if err != nil {
		return err
	}

	opts := armrpc_controller.Options{
		DatabaseClient: db,
	}

	var ctrl armrpc_controller.Controller
	switch strings.ToLower(resourceID.ScopeSegments()[0].Type) {
	case "aws":
		ctrl, err = defaultoperation.NewDefaultSyncPut(opts,
			armrpc_controller.ResourceOptions[datamodel.AWSPlane]{
				RequestConverter:  converter.AWSPlaneDataModelFromVersioned,
				ResponseConverter: converter.AWSPlaneDataModelToVersioned,
			})

	case "azure":
		ctrl, err = defaultoperation.NewDefaultSyncPut(opts,
			armrpc_controller.ResourceOptions[datamodel.AzurePlane]{
				RequestConverter:  converter.AzurePlaneDataModelFromVersioned,
				ResponseConverter: converter.AzurePlaneDataModelToVersioned,
			})

	case "radius":
		ctrl, err = defaultoperation.NewDefaultSyncPut(opts,
			armrpc_controller.ResourceOptions[datamodel.RadiusPlane]{
				RequestConverter:  converter.RadiusPlaneDataModelFromVersioned,
				ResponseConverter: converter.RadiusPlaneDataModelToVersioned,
			})

	default:
		err = fmt.Errorf("invalid plane type: %s", resourceID.ScopeSegments()[0].Type)
	}
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
	rpcContext, err := v1.FromARMRequest(request, s.options.Config.Server.PathBase, s.options.Config.Environment.RoleLocation)
	if err != nil {
		return err
	}
	wrappedCtx := v1.WithARMRequestContext(ctx, rpcContext)

	_, err = ctrl.Run(wrappedCtx, nil, request)
	if err != nil {
		return err
	}

	return nil
}

// Run sets up a server to listen on a given address, and shuts it down when the context is done. It returns an
// error if the server fails to start or stops unexpectedly.
func (s *Service) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	logger := ucplog.FromContextOrDiscard(ctx)
	service, err := s.Initialize(ctx)
	if err != nil {
		return err
	}
	if s.userAuthorization != nil {
		done := make(chan struct{})
		go func() {
			defer close(done)
			if err := s.userAuthorization.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error(err, "user authorization cache stopped")
			}
		}()
		defer func() {
			cancel()
			<-done
		}()
	}

	// Handle shutdown based on the context
	go func() {
		<-ctx.Done()
		// We don't care about shutdown errors
		_ = service.Shutdown(ctx)
	}()

	internalErr := make(chan error, 1)
	if s.options.Config.Authorization.EffectiveMode() != authz.ModeOff {
		if s.options.Config.Server.InternalPort == 0 {
			return errors.New("server.internalPort is required when the authorization mode is not off")
		}
		internal := s.internalServer(service)
		go func() {
			<-ctx.Done()
			_ = internal.Shutdown(ctx)
		}()
		go func() {
			logger.Info(fmt.Sprintf("listening for internal traffic on: '%s'...", internal.Addr))
			err := mtls.ListenAndServe(ctx, internal, s.options.Config.Authorization, nil)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				internalErr <- fmt.Errorf("internal endpoint failed: %w", err)
				_ = service.Shutdown(ctx)
			}
		}()
	}

	if s.options.Config.Authorization.EffectiveMode() != authz.ModeOff {
		// A6: ask for the aggregation proxy's client certificate. It is verified against the
		// cluster's request header CA before X-Remote-* headers are trusted.
		service.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ClientAuth: tls.RequestClientCert}
	}

	logger.Info(fmt.Sprintf("listening on: '%s'...", s.options.Config.Server.Address()))
	if s.options.Config.Server.TLSCertificateDirectory == "" {
		err = service.ListenAndServe()
	} else {
		err = service.ListenAndServeTLS(s.options.Config.Server.TLSCertificateDirectory+"/tls.crt", s.options.Config.Server.TLSCertificateDirectory+"/tls.key")
	}

	select {
	case ierr := <-internalErr:
		return ierr
	default:
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

// internalServer returns a server for internal component traffic. It serves the same
// routes as the aggregation endpoint on InternalPort, so the Kubernetes API aggregation
// endpoint and its certificate are unchanged.
func (s *Service) internalServer(aggregation *http.Server) *http.Server {
	return &http.Server{
		Addr:        fmt.Sprintf("%s:%d", s.options.Config.Server.Host, s.options.Config.Server.InternalPort),
		Handler:     authzcheck.MarkInternal(aggregation.Handler),
		BaseContext: aggregation.BaseContext,
	}
}
