// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	hostOpts "github.com/project-radius/radius/pkg/armrpc/hostoptions"
	"github.com/project-radius/radius/pkg/kubeutil"
	metricsprovider "github.com/project-radius/radius/pkg/metrics/provider"
	metricsservice "github.com/project-radius/radius/pkg/metrics/service"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/pkg/trace"
	"github.com/project-radius/radius/pkg/ucp/backend"
	"github.com/project-radius/radius/pkg/ucp/config"
	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	qprovider "github.com/project-radius/radius/pkg/ucp/queue/provider"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/ucplog"

	kube_rest "k8s.io/client-go/rest"
)

const (
	HTTPServerStopTimeout = time.Second * 10
	ServiceName           = "ucp"
)

type Options struct {
	Port                   string
	StorageProviderOptions dataprovider.StorageProviderOptions
	LoggingOptions         ucplog.LoggingOptions
	SecretProviderOptions  provider.SecretProviderOptions
	QueueProviderOptions   qprovider.QueueProviderOptions
	MetricsProviderOptions metricsprovider.MetricsProviderOptions
	TracerProviderOptions  trace.Options
	TLSCertDir             string
	BasePath               string
	InitialPlanes          []rest.Plane
	Identity               hostoptions.Identity
	UCPConnection          sdk.Connection
	Location               string
}

const UCPProviderName = "ucp"

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
	qproviderOpts := opts.Config.QueueProvider
	metricsOpts := opts.Config.MetricsProvider
	traceOpts := opts.Config.TracerProvider
	loggingOpts := opts.Config.Logging
	identity := opts.Config.Identity
	// Set the default authentication method if AuthMethod is not set.
	if identity.AuthMethod == "" {
		identity.AuthMethod = hostoptions.AuthDefault
	}

	location := opts.Config.Location
	if location == "" {
		location = "global"
	}

	var cfg *kube_rest.Config
	if opts.Config.UCP.Kind == config.UCPConnectionKindKubernetes {
		cfg, err = kubeutil.NewClientConfig(&kubeutil.ConfigOptions{
			// TODO: Allow to use custom context via configuration. - https://github.com/project-radius/radius/issues/5433
			ContextName: "",
			QPS:         kubeutil.ServerQPS,
			Burst:       kubeutil.ServerBurst,
		})
		if err != nil {
			return Options{}, fmt.Errorf("failed to get kubernetes config: %w", err)
		}
	}

	ucpConn, err := config.NewConnectionFromUCPConfig(&opts.Config.UCP, cfg)
	if err != nil {
		return Options{}, err
	}

	return Options{
		Port:                   port,
		TLSCertDir:             tlsCertDir,
		BasePath:               basePath,
		StorageProviderOptions: storeOpts,
		SecretProviderOptions:  secretOpts,
		QueueProviderOptions:   qproviderOpts,
		MetricsProviderOptions: metricsOpts,
		TracerProviderOptions:  traceOpts,
		LoggingOptions:         loggingOpts,
		InitialPlanes:          planes,
		Identity:               identity,
		UCPConnection:          ucpConn,
		Location:               location,
	}, nil
}

func NewServer(options *Options) (*hosting.Host, error) {
	var enableAsyncWorker bool
	flag.BoolVar(&enableAsyncWorker, "enable-asyncworker", true, "Flag to run async request process worker (for private preview and dev/test purpose).")

	hostingServices := []hosting.Service{
		api.NewService(api.ServiceOptions{
			ProviderName:           UCPProviderName,
			Address:                ":" + options.Port,
			TLSCertDir:             options.TLSCertDir,
			BasePath:               options.BasePath,
			StorageProviderOptions: options.StorageProviderOptions,
			SecretProviderOptions:  options.SecretProviderOptions,
			QueueProviderOptions:   options.QueueProviderOptions,
			InitialPlanes:          options.InitialPlanes,
			Identity:               options.Identity,
			UCPConnection:          options.UCPConnection,
			Location:               options.Location,
		}),
	}

	if options.StorageProviderOptions.Provider == dataprovider.TypeETCD &&
		options.StorageProviderOptions.ETCD.InMemory {
		hostingServices = append(hostingServices, data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{ClientConfigSink: options.StorageProviderOptions.ETCD.Client}))
	}

	options.MetricsProviderOptions.ServiceName = ServiceName
	if options.MetricsProviderOptions.Prometheus.Enabled {
		metricOptions := metricsservice.HostOptions{
			Config: &options.MetricsProviderOptions,
		}
		hostingServices = append(hostingServices, metricsservice.NewService(metricOptions))
	}

	if enableAsyncWorker {
		backendServiceOptions := hostOpts.HostOptions{
			Config: &hostOpts.ProviderConfig{
				StorageProvider: options.StorageProviderOptions,
				SecretProvider:  options.SecretProviderOptions,
				QueueProvider:   options.QueueProviderOptions,
				MetricsProvider: options.MetricsProviderOptions,
				TracerProvider:  options.TracerProviderOptions,
			},
		}
		hostingServices = append(hostingServices, backend.NewService(backendServiceOptions))
	}

	return &hosting.Host{
		Services: hostingServices,
	}, nil
}
