// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package server

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/hostoptions"
	"github.com/project-radius/radius/pkg/ucp/rest"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	HTTPServerStopTimeout = time.Second * 10
)

type Options struct {
	Port                   string
	UCPHandler             ucphandler.UCPHandler
	DBClient               store.StorageClient
	StorageProviderOptions dataprovider.StorageProviderOptions
	TLSCertDir             string
	BasePath               string
	InitialPlanes          []rest.Plane
}

func NewServerOptionsFromEnvironment() (Options, error) {
	basePath, ok := os.LookupEnv("BASE_PATH")
	if ok && len(basePath) > 0 && (!strings.HasPrefix(basePath, "/") || strings.HasSuffix(basePath, "/")) {
		return Options{}, errors.New("env: BASE_PATH must begin with '/' and must not end with '/'")
	}

	tlsCertDir := os.Getenv("TLS_CERT_DIR")
	ucpConfigFile := os.Getenv("UCP_CONFIG")
	port := os.Getenv("PORT")

	opts, err := hostoptions.NewHostOptionsFromEnvironment(ucpConfigFile)
	if err != nil {
		return Options{}, err
	}

	storeOpts := opts.Config.StorageProvider
	planes := opts.Config.Planes

	return Options{
		Port:                   port,
		TLSCertDir:             tlsCertDir,
		BasePath:               basePath,
		StorageProviderOptions: storeOpts,
		InitialPlanes:          planes,
	}, nil
}

func NewServer(options Options) (*hosting.Host, error) {
	clientconfigSource := hosting.NewAsyncValue()
	hostingServices := []hosting.Service{
		api.NewService(api.ServiceOptions{
			Address: ":" + options.Port,
			UcpHandler: ucphandler.NewUCPHandler(ucphandler.UCPHandlerOptions{
				BasePath: options.BasePath,
			}),
			DBClient:               options.DBClient,
			ClientConfigSource:     clientconfigSource,
			TLSCertDir:             options.TLSCertDir,
			BasePath:               options.BasePath,
			StorageProviderOptions: options.StorageProviderOptions,
			InitialPlanes:          options.InitialPlanes,
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

	return &hosting.Host{
		Services: hostingServices,
	}, nil

}
