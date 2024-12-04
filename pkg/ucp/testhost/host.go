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

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/hostoptions"
	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp"
	"github.com/radius-project/radius/pkg/ucp/api/v20231001preview"
	"github.com/radius-project/radius/pkg/ucp/config"
	"github.com/radius-project/radius/pkg/ucp/dataprovider"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	queue "github.com/radius-project/radius/pkg/ucp/queue/client"
	queueprovider "github.com/radius-project/radius/pkg/ucp/queue/provider"
	"github.com/radius-project/radius/pkg/ucp/secret"
	secretprovider "github.com/radius-project/radius/pkg/ucp/secret/provider"
	"github.com/radius-project/radius/pkg/ucp/server"
	"github.com/radius-project/radius/pkg/ucp/store"
	"github.com/radius-project/radius/test/integrationtest/testhost"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestHostOption can be used to configure the UCP options before the server is started.
type TestHostOption interface {
	// Apply applies the configuration to the UCP options.
	Apply(options *ucp.Options)
}

// TestHostOptionFunc is a function that implements the TestHostOption interface.
type TestHostOptionFunc func(options *ucp.Options)

// Apply applies the function to the UCP options.
func (f TestHostOptionFunc) Apply(options *ucp.Options) {
	f(options)
}

// NoModules is a TestHostOption that disables all UCP modules.
func NoModules() TestHostOptionFunc {
	return func(options *ucp.Options) {
		options.Modules = []modules.Initializer{}
	}
}

// TestServerMocks provides access to mock instances created by the TestServer.
type TestServerMocks struct {
	// DataProvider is the mock data provider.
	DataProvider *dataprovider.MockDataStorageProvider

	// QueueProvider is the mock queue provider.
	QueueProvider *queueprovider.QueueProvider

	// SecretProvider is the mock secret provider.
	SecretProvider *secretprovider.SecretProvider

	// Secrets is the mock secret client.
	Secrets *secret.MockClient

	// StatusManager is the mock status manager.
	StatusManager *statusmanager.MockStatusManager

	// Storage is the mock storage client.
	Storage *store.MockStorageClient
}

// NewMocks creates a new set of mocks for the test server.
func NewMocks(t *testing.T) *TestServerMocks {
	ctrl := gomock.NewController(t)
	dataClient := store.NewMockStorageClient(ctrl)
	dataProvider := dataprovider.NewMockDataStorageProvider(ctrl)
	dataProvider.EXPECT().
		GetStorageClient(gomock.Any(), gomock.Any()).
		Return(dataClient, nil).
		AnyTimes()

	queueClient := queue.NewMockClient(ctrl)
	queueProvider := queueprovider.New(queueprovider.QueueProviderOptions{Name: "System.Resources"})
	queueProvider.SetClient(queueClient)

	secretClient := secret.NewMockClient(ctrl)
	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secretClient)

	statusManager := statusmanager.NewMockStatusManager(ctrl)
	return &TestServerMocks{
		DataProvider:   dataProvider,
		QueueProvider:  queueProvider,
		SecretProvider: secretProvider,
		Secrets:        secretClient,
		StatusManager:  statusManager,
		Storage:        dataClient,
	}
}

// Apply updates the UCP options to use the mocks.
func (m *TestServerMocks) Apply(options *ucp.Options) {
	options.SecretProvider = m.SecretProvider
	options.StorageProvider = m.DataProvider
	options.QueueProvider = m.QueueProvider
	options.StatusManager = m.StatusManager
}

// TestHost provides a test host for the UCP server.
type TestHost struct {
	*testhost.TestHost
	options *ucp.Options

	clientFactoryUCP *v20231001preview.ClientFactory
}

// Internals provides access to the internal options of the server. This allows tests
// to access the data stores and manipulate the server state.
func (th *TestHost) Options() *ucp.Options {
	return th.options
}

// UCP provides access to the generated clients for the UCP API.
func (ts *TestHost) UCP() *v20231001preview.ClientFactory {
	if ts.clientFactoryUCP == nil {
		connection, err := sdk.NewDirectConnection(ts.BaseURL())
		require.NoError(ts.T(), err)

		ts.clientFactoryUCP, err = v20231001preview.NewClientFactory(&aztoken.AnonymousCredential{}, sdk.NewClientOptions(connection))
		require.NoError(ts.T(), err)
	}

	return ts.clientFactoryUCP
}

// Start creates and starts a new TestServer.
func Start(t *testing.T, opts ...TestHostOption) *TestHost {
	config := &ucp.Config{
		Environment: hostoptions.EnvironmentOptions{
			Name:         "test",
			RoleLocation: v1.LocationGlobal,
		},
		Queue: queueprovider.QueueProviderOptions{
			Provider: queueprovider.TypeInmemory,
			Name:     "ucp",
		},
		Secrets: secretprovider.SecretProviderOptions{
			Provider: secretprovider.TypeInMemorySecret,
		},
		Server: hostoptions.ServerOptions{
			// Initialized dynamically when the server is started.
		},
		Storage: dataprovider.StorageProviderOptions{
			Provider: dataprovider.TypeInMemory,
		},
		UCP: config.UCPOptions{
			Kind: config.UCPConnectionKindDirect,
			Direct: &config.UCPDirectConnectionOptions{
				Endpoint: "http://localhost:65000", // Initialized dynamically when the server is started.
			},
		},
	}

	options, err := ucp.NewOptions(context.Background(), config)
	require.NoError(t, err)

	for _, opt := range opts {
		opt.Apply(options)
	}

	return StartWithOptions(t, options)

}

func StartWithOptions(t *testing.T, options *ucp.Options) *TestHost {
	options.Config.Server.Host = "localhost"
	if options.Config.Server.Port == 0 {
		options.Config.Server.Port = testhost.AllocateFreePort(t)
	}

	baseURL := fmt.Sprintf(
		"http://%s%s",
		options.Config.Server.Address(),
		options.Config.Server.PathBase)
	baseURL = strings.TrimSuffix(baseURL, "/")

	options.Config.UCP.Kind = config.UCPConnectionKindDirect
	options.Config.UCP.Direct = &config.UCPDirectConnectionOptions{Endpoint: baseURL}

	// Instantiate the UCP client now that we know the URL.
	var err error
	options.UCP, err = sdk.NewDirectConnection(baseURL)
	require.NoError(t, err)

	host, err := server.NewServer(options)
	require.NoError(t, err, "failed to create server")

	return &TestHost{
		TestHost: testhost.StartHost(t, host, baseURL),
		options:  options,
	}
}
