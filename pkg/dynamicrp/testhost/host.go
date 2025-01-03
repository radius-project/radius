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

package testhost

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/kubernetesclient/kubernetesclientprovider"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/components/testhost"
	"github.com/radius-project/radius/pkg/dynamicrp"
	"github.com/radius-project/radius/pkg/dynamicrp/server"
	"github.com/radius-project/radius/pkg/recipes/driver"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/config"
	ucptesthost "github.com/radius-project/radius/pkg/ucp/testhost"
	"github.com/stretchr/testify/require"
)

// TestHostOptions supports configuring the dynamic-rp test host.
type TestHostOption interface {
	// Apply applies the option to the dynamic-rp options.
	Apply(options *dynamicrp.Options)
}

// TestHostOptionFunc is a function that implements the TestHostOption interface.
type TestHostOptionFunc func(options *dynamicrp.Options)

// Apply applies the function to the dynamic-rp options.
func (f TestHostOptionFunc) Apply(options *dynamicrp.Options) {
	f(options)
}

// TestHost provides a test host for the dynamic-rp server.
type TestHost struct {
	*testhost.TestHost
	options *dynamicrp.Options
}

func (th *TestHost) Options() *dynamicrp.Options {
	return th.options
}

func Start(t *testing.T, opts ...TestHostOption) (*TestHost, *ucptesthost.TestHost) {
	config := &dynamicrp.Config{
		Database: databaseprovider.Options{
			Provider: databaseprovider.TypeInMemory,
		},
		Environment: hostoptions.EnvironmentOptions{
			Name:         "test",
			RoleLocation: v1.LocationGlobal,
		},
		Kubernetes: kubernetesclientprovider.Options{
			Kind: kubernetesclientprovider.KindNone,
		},
		Queue: queueprovider.QueueProviderOptions{
			Provider: queueprovider.TypeInmemory,
			Name:     "dynamic-rp",
		},
		Secrets: secretprovider.SecretProviderOptions{
			Provider: secretprovider.TypeInMemorySecret,
		},
		Server: hostoptions.ServerOptions{
			// Initialized dynamically when the server is started.
		},
		UCP: config.UCPOptions{
			Kind: config.UCPConnectionKindDirect,
			Direct: &config.UCPDirectConnectionOptions{
				Endpoint: "http://localhost:65000", // Initialized dynamically when the server is started.
			},
		},
	}

	options, err := dynamicrp.NewOptions(context.Background(), config)
	require.NoError(t, err)

	// Prevent the default recipe drivers from being registered.
	options.Recipes.Drivers = map[string]func(options *dynamicrp.Options) (driver.Driver, error){}

	for _, opt := range opts {
		opt.Apply(options)
	}

	return StartWithOptions(t, options)
}

// StartWithOptions uses the provided options to start the dynamic-rp test host and an instance of UCP
// configured to route traffic to the dynamic-rp test host.
//
// Manually configuring the server information other than the port is not supported.
func StartWithOptions(t *testing.T, options *dynamicrp.Options) (*TestHost, *ucptesthost.TestHost) {
	options.Config.Server.Host = "localhost"
	options.Config.Server.PathBase = "/" + uuid.New().String()
	if options.Config.Server.Port == 0 {
		options.Config.Server.Port = testhost.AllocateFreePort(t)
	}

	// Allocate a port for UCP.
	ucpPort := testhost.AllocateFreePort(t)
	options.Config.UCP.Kind = config.UCPConnectionKindDirect
	options.Config.UCP.Direct = &config.UCPDirectConnectionOptions{Endpoint: fmt.Sprintf("http://localhost:%d", ucpPort)}

	var err error
	options.UCP, err = sdk.NewDirectConnection(options.Config.UCP.Direct.Endpoint)
	require.NoError(t, err)

	baseURL := fmt.Sprintf(
		"http://%s%s",
		options.Config.Server.Address(),
		options.Config.Server.PathBase)
	baseURL = strings.TrimSuffix(baseURL, "/")

	host, err := server.NewServer(options)
	require.NoError(t, err, "failed to create server")

	th := testhost.StartHost(t, host, baseURL)
	return &TestHost{TestHost: th, options: options}, startUCP(t, baseURL, ucpPort)
}

func startUCP(t *testing.T, dynamicRPURL string, ucpPort int) *ucptesthost.TestHost {
	return ucptesthost.Start(t, ucptesthost.TestHostOptionFunc(func(options *ucp.Options) {
		// Initialize UCP with its listening port
		options.Config.Server.Port = ucpPort

		// Intitialize UCP with the dynamic-rp URL
		options.Config.Routing.DefaultDownstreamEndpoint = dynamicRPURL
	}))
}
