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
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/ucphandler"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	"github.com/project-radius/radius/pkg/ucp/store"
)

const (
	HTTPServerStopTimeout = time.Second * 10
)

type Options struct {
	Port                    string
	UCPHandler              ucphandler.UCPHandler
	DBClient                store.StorageClient
	TLSCertDir              string
	BasePath                string
	DefaultPlanesConfigFile string
}

func NewServerOptionsFromEnvironment() (Options, error) {
	basePath, ok := os.LookupEnv("BASE_PATH")
	if ok && len(basePath) > 0 && (!strings.HasPrefix(basePath, "/") || strings.HasSuffix(basePath, "/")) {
		return Options{}, errors.New("env: BASE_PATH must begin with '/' and must not end with '/'")
	}

	tlsCertDir := os.Getenv("TLS_CERT_DIR")

	defaultPlanesConfigFile := os.Getenv("DEFAULT_PLANES_CONFIG")

	port := os.Getenv("PORT")

	return Options{
		Port:                    port,
		TLSCertDir:              tlsCertDir,
		BasePath:                basePath,
		DefaultPlanesConfigFile: defaultPlanesConfigFile,
	}, nil
}

func NewServer(options Options) (*hosting.Host, error) {
	clientconfigSource := hosting.NewAsyncValue()

	return &hosting.Host{
		Services: []hosting.Service{
			data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{
				ClientConfigSink: clientconfigSource,
			}),
			api.NewService(api.ServiceOptions{
				Address: ":" + options.Port,
				UcpHandler: ucphandler.NewUCPHandler(ucphandler.UCPHandlerOptions{
					BasePath: options.BasePath,
				}),
				DBClient:                options.DBClient,
				ClientConfigSource:      clientconfigSource,
				DefaultPlanesConfigFile: options.DefaultPlanesConfigFile,
				TLSCertDir:              options.TLSCertDir,
				BasePath:                options.BasePath,
			}),
		},
	}, nil
}
