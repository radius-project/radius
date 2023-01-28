// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/project-radius/radius/pkg/rp/kube"
	"github.com/project-radius/radius/pkg/sdk"
	metricsprovider "github.com/project-radius/radius/pkg/telemetry/metrics/provider"
	metricsservice "github.com/project-radius/radius/pkg/telemetry/metrics/service"
	metricsservicehostoptions "github.com/project-radius/radius/pkg/telemetry/metrics/service/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	etcdclient "go.etcd.io/etcd/client/v3"
	kube_rest "k8s.io/client-go/rest"
)

const (
	HTTPServerStopTimeout = time.Second * 10
)

type Options struct {
	Port                   string
	StorageProviderOptions dataprovider.StorageProviderOptions
	LoggingOptions         ucplog.LoggingOptions
	SecretProviderOptions  provider.SecretProviderOptions
	MetricsProviderOptions metricsprovider.MetricsProviderOptions
	TLSCertDir             string
	BasePath               string
	InitialPlanes          []rest.Plane
	Identity               hostoptions.Identity
	UCPConnection          sdk.Connection
}

func NewServerOptionsFromEnvironment() (Options, error) {
	basePath, ok := os.LookupEnv("BASE_PATH")
	if ok && len(basePath) > 0 && (!strings.HasPrefix(basePath, "/") || strings.HasSuffix(basePath, "/")) {
		return Options{}, errors.New("env: BASE_PATH must begin with '/' and must not end with '/'")
	}

	tlsCertDir := os.Getenv("TLS_CERT_DIR")
	ucpConfigFile := os.Getenv("UCP_CONFIG")

	port := os.Getenv("PORT")
	if port == "" {
		return Options{}, errors.New("UCP Port number must be set")
	}

	opts, err := hostoptions.NewHostOptionsFromEnvironment(ucpConfigFile)
	if err != nil {
		return Options{}, err
	}

	storeOpts := opts.Config.StorageProvider
	planes := opts.Config.Planes
	secretOpts := opts.Config.SecretProvider
	metricsOpts := opts.Config.MetricsProvider
	loggingOpts := opts.Config.Logging
	identity := opts.Config.Identity
	if identity.Auth == "" {
		identity.Auth = hostoptions.AuthDefault
	}

	var cfg *kube_rest.Config
	if opts.Config.UCP.Kind == sdk.UCPConnectionKindKubernetes {
		cfg, err = kube.GetConfig()
		if err != nil {
			return Options{}, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	ucpConn, err := sdk.GetUCPConnection(&opts.Config.UCP, cfg)
	if err != nil {
		return Options{}, err
	}

	return Options{
		Port:                   port,
		TLSCertDir:             tlsCertDir,
		BasePath:               basePath,
		StorageProviderOptions: storeOpts,
		SecretProviderOptions:  secretOpts,
		MetricsProviderOptions: metricsOpts,
		LoggingOptions:         loggingOpts,
		InitialPlanes:          planes,
		Identity:               identity,
		UCPConnection:          ucpConn,
	}, nil
}

func NewServer(options Options) (*hosting.Host, error) {
	clientconfigSource := hosting.NewAsyncValue[etcdclient.Client]()
	hostingServices := []hosting.Service{
		api.NewService(api.ServiceOptions{
			Address:                ":" + options.Port,
			ClientConfigSource:     clientconfigSource,
			TLSCertDir:             options.TLSCertDir,
			BasePath:               options.BasePath,
			StorageProviderOptions: options.StorageProviderOptions,
			SecretProviderOptions:  options.SecretProviderOptions,
			InitialPlanes:          options.InitialPlanes,
			Identity:               options.Identity,
			UCPConnection:          options.UCPConnection,
		}),
	}

	if options.StorageProviderOptions.Provider == dataprovider.TypeETCD &&
		options.StorageProviderOptions.ETCD.InMemory {
		// For in-memory etcd we need to register another service to manage its lifecycle.
		//
		// The client will be initialized asynchronously.

		options.StorageProviderOptions.ETCD.Client = clientconfigSource
		hostingServices = append(hostingServices, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: clientconfigSource}))
	}

	if options.MetricsProviderOptions.Prometheus.Enabled {
		metricOptions := metricsservicehostoptions.HostOptions{
			Config: &options.MetricsProviderOptions,
		}
		hostingServices = append(hostingServices, metricsservice.NewService(metricOptions))
	}

	return &hosting.Host{
		Services: hostingServices,
	}, nil
}
