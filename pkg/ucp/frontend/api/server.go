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

	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	aztoken "github.com/project-radius/radius/pkg/azure/tokencredentials"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/sdk"
	ucpaws "github.com/project-radius/radius/pkg/ucp/aws"
	sdk_cred "github.com/project-radius/radius/pkg/ucp/credentials"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller"
	planes_ctrl "github.com/project-radius/radius/pkg/ucp/frontend/controller/planes"
	"github.com/project-radius/radius/pkg/ucp/frontend/versions"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/secret"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/store"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/go-logr/logr"
	"github.com/gorilla/mux"
	etcdclient "go.etcd.io/etcd/client/v3"
)

const (
	DefaultPlanesConfig = "DEFAULT_PLANES_CONFIG"
)

type ServiceOptions struct {
	Address                 string
	ClientConfigSource      *hosting.AsyncValue[etcdclient.Client]
	Configure               func(*mux.Router)
	TLSCertDir              string
	DefaultPlanesConfigFile string
	UCPConfigFile           string
	BasePath                string
	StorageProviderOptions  dataprovider.StorageProviderOptions
	SecretProviderOptions   provider.SecretProviderOptions
	InitialPlanes           []rest.Plane
	Identity                hostoptions.Identity
	UCPConnection           sdk.Connection
}

type Service struct {
	options         ServiceOptions
	storageProvider dataprovider.DataStorageProvider
	secretProvider  *provider.SecretProvider
	secretClient    secret.Client
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

func (s *Service) newAWSConfig(ctx context.Context) (aws.Config, error) {
	logger := logr.FromContextOrDiscard(ctx)
	credProviders := []func(*config.LoadOptions) error{}

	switch s.options.Identity.AuthMethod {
	case hostoptions.AuthUCPCredential:
		provider, err := sdk_cred.NewAWSCredentialProvider(s.secretProvider, s.options.UCPConnection, &aztoken.AnonymousCredential{})
		if err != nil {
			return aws.Config{}, err
		}
		p := ucpaws.NewUCPCredentialProvider(provider, ucpaws.DefaultExpireDuration)
		credProviders = append(credProviders, config.WithCredentialsProvider(p))
		logger.Info("Configuring 'UCPCredential' authentication mode using UCP Credential API")

	default:
		logger.Info("Configuring default authentication mode with environment variable.")
	}

	awscfg, err := config.LoadDefaultConfig(ctx, credProviders...)
	if err != nil {
		return aws.Config{}, err
	}

	return awscfg, nil
}

func (s *Service) Initialize(ctx context.Context) (*http.Server, error) {
	r := mux.NewRouter()

	s.storageProvider = s.initializeStorageProvider(ctx)

	// TODO: this is used EVERYWHERE right now. We'd like to pass
	// around storage provider instead but will have to refactor
	// tons of stuff.
	db, err := s.storageProvider.GetStorageClient(ctx, "ucp")
	if err != nil {
		return nil, err
	}

	if err = s.initializeSecretClient(ctx); err != nil {
		return nil, err
	}

	awscfg, err := s.newAWSConfig(ctx)
	if err != nil {
		return nil, err
	}

	ctrlOpts := ctrl.Options{
		BasePath:     s.options.BasePath,
		DB:           db,
		SecretClient: s.secretClient,
		Address:      s.options.Address,

		AWSCloudControlClient:   cloudcontrol.NewFromConfig(awscfg),
		AWSCloudFormationClient: cloudformation.NewFromConfig(awscfg),

		CommonControllerOptions: armrpc_controller.Options{
			DataProvider: s.storageProvider,

			// TODO: These fields are not used in UCP. We'd like to unify these
			// options types eventually, but that will take some time.
			SecretClient:  nil,
			KubeClient:    nil,
			StatusManager: nil,
		},
	}

	err = Register(ctx, r, ctrlOpts)
	if err != nil {
		return nil, err
	}

	if s.options.Configure != nil {
		s.options.Configure(r)
	}

	err = s.configureDefaultPlanes(ctx, db, s.options.InitialPlanes)
	if err != nil {
		return nil, err
	}

	app := http.Handler(r)
	app = middleware.UseLogValues(app, s.options.BasePath)
	app = servicecontext.ARMRequestCtx(s.options.BasePath, "global")(app)

	server := &http.Server{
		Addr: s.options.Address,
		// Need to be able to respond to requests with planes and resourcegroups segments with any casing e.g.: /Planes, /resourceGroups
		// AWS SDK is case sensitive. Therefore, cannot use lowercase middleware. Therefore, introducing a new middleware that translates
		// the path for only these segments and preserves the case for the other parts of the path.
		// TODO: Once https://github.com/project-radius/radius/issues/3582 is fixed, we could use the lowercase middleware
		Handler: middleware.NormalizePath(app),
		BaseContext: func(ln net.Listener) context.Context {
			return ctx
		},
	}
	return server, nil
}

func (s *Service) initializeStorageProvider(ctx context.Context) dataprovider.DataStorageProvider {
	if s.options.StorageProviderOptions.Provider == dataprovider.TypeETCD {
		s.options.StorageProviderOptions.ETCD.Client = s.options.ClientConfigSource
	}

	return dataprovider.NewStorageProvider(s.options.StorageProviderOptions)
}

// initializeSecretClient initializes secret client on server startup.
func (s *Service) initializeSecretClient(ctx context.Context) error {
	if s.options.SecretProviderOptions.Provider == provider.TypeETCDSecret {
		s.options.SecretProviderOptions.ETCD.Client = s.options.ClientConfigSource
	}
	s.secretProvider = provider.NewSecretProvider(s.options.SecretProviderOptions)

	var err error
	s.secretClient, err = s.secretProvider.GetClient(ctx)
	if err != nil {
		return err
	}
	return nil
}

// configureDefaultPlanes reads the configuration file specified by the env var to configure default planes into UCP
func (s *Service) configureDefaultPlanes(ctx context.Context, dbClient store.StorageClient, planes []rest.Plane) error {
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

		// Using the latest API version to make a request to configure the default planes
		url := fmt.Sprintf("%s?api-version=%s", plane.ID, versions.DefaultAPIVersion)
		request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(body))
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
