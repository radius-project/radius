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

package testserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client/v3"
	"go.uber.org/mock/gomock"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	backend_ctrl "github.com/radius-project/radius/pkg/armrpc/asyncoperation/controller"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/statusmanager"
	"github.com/radius-project/radius/pkg/armrpc/asyncoperation/worker"
	"github.com/radius-project/radius/pkg/armrpc/rpctest"
	"github.com/radius-project/radius/pkg/armrpc/servicecontext"
	"github.com/radius-project/radius/pkg/components/database"
	"github.com/radius-project/radius/pkg/components/database/databaseprovider"
	"github.com/radius-project/radius/pkg/components/queue"
	"github.com/radius-project/radius/pkg/components/queue/queueprovider"
	"github.com/radius-project/radius/pkg/components/secret"
	"github.com/radius-project/radius/pkg/components/secret/secretprovider"
	"github.com/radius-project/radius/pkg/middleware"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/backend"
	"github.com/radius-project/radius/pkg/ucp/data"
	"github.com/radius-project/radius/pkg/ucp/frontend/api"
	"github.com/radius-project/radius/pkg/ucp/frontend/modules"
	"github.com/radius-project/radius/pkg/ucp/hosting"
	"github.com/radius-project/radius/pkg/ucp/hostoptions"
	"github.com/radius-project/radius/pkg/ucp/server"
	"github.com/radius-project/radius/pkg/validator"
	"github.com/radius-project/radius/swagger"
	"github.com/radius-project/radius/test/testcontext"
)

// NoModules can be used to start a test server without any modules. This is useful for testing the server itself and core functionality
// like planes.
func NoModules(options modules.Options) []modules.Initializer {
	return nil
}

// TestServer can run a UCP server using the Go httptest package. It provides access to an isolated ETCD instances for storage
// of resources and secrets. Alteratively, it can also be used with gomock.
//
// Do not create a TestServer directly, use StartWithETCD or StartWithMocks instead.
type TestServer struct {
	// BaseURL is the base URL of the server, including the path base.
	BaseURL string

	// Clients gets access to the clients created by TestServer regardless of whether
	// they are mocks.
	Clients *TestServerClients

	// Mocks gets access to the mock clients. Will be nil if StartWithETCD is used.
	Mocks *TestServerMocks

	// Server provides access to the test HTTP server.
	Server *httptest.Server

	cancel      context.CancelFunc
	etcdService *data.EmbeddedETCDService
	etcdClient  *etcdclient.Client
	t           *testing.T
	stoppedChan <-chan struct{}
	shutdown    sync.Once
}

// TestServerClients provides access to the clients created by the TestServer.
type TestServerClients struct {
	// QueueProvider is the queue client provider.
	QueueProvider *queueprovider.QueueProvider

	// SecretProvider is the secret client provider.
	SecretProvider *secretprovider.SecretProvider

	// DatabaseProvider is the database client provider.
	DatabaseProvider *databaseprovider.DatabaseProvider
}

// TestServerMocks provides access to mock instances created by the TestServer.
type TestServerMocks struct {
	// Secrets is the mock secret client.
	Secrets *secret.MockClient

	// Database is the mock database client.
	Database *database.MockClient
}

// Client provides access to an http.Client that can be used to send requests. Most tests should use the functionality
// like MakeRequest instead of testing the client directly.
func (ts *TestServer) Client() *http.Client {
	return ts.Server.Client()
}

// Close shuts down the server and will block until shutdown completes.
func (ts *TestServer) Close() {
	// We're being picking about resource cleanup here, because unless we are picky we hit scalability
	// problems in tests pretty quickly.
	ts.shutdown.Do(func() {
		ts.cancel()       // Start ETCD shutdown
		ts.Server.Close() // Stop HTTP server

		if ts.etcdClient != nil {
			ts.etcdClient.Close() // Stop ETCD Client
		}

		if ts.stoppedChan != nil {
			<-ts.stoppedChan // ETCD stopped
		}
	})
}

// StartWithMocks creates and starts a new TestServer that used an mocks for storage.
func StartWithMocks(t *testing.T, configureModules func(options modules.Options) []modules.Initializer) *TestServer {
	ctx, cancel := testcontext.NewWithCancel(t)

	// Generate a random base path to ensure we're handling it correctly.
	pathBase := "/" + uuid.New().String()

	ctrl := gomock.NewController(t)
	databaseClient := database.NewMockClient(ctrl)
	databaseProvider := databaseprovider.FromClient(databaseClient)

	queueClient := queue.NewMockClient(ctrl)
	queueProvider := queueprovider.New(queueprovider.QueueProviderOptions{Name: "System.Resources"})
	queueProvider.SetClient(queueClient)

	secretClient := secret.NewMockClient(ctrl)
	secretProvider := secretprovider.NewSecretProvider(secretprovider.SecretProviderOptions{})
	secretProvider.SetClient(secretClient)

	statusManager := statusmanager.NewMockStatusManager(ctrl)

	router := chi.NewRouter()
	router.Use(servicecontext.ARMRequestCtx(pathBase, "global"))

	app := http.Handler(router)
	app = middleware.NormalizePath(app)
	server := httptest.NewUnstartedServer(app)
	server.Config.BaseContext = func(l net.Listener) context.Context {
		return ctx
	}

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, []string{pathBase}, "")
	require.NoError(t, err, "failed to load OpenAPI spec")

	options := modules.Options{
		Address:          "localhost:9999", // Will be dynamically populated when server is started
		PathBase:         pathBase,
		Config:           &hostoptions.UCPConfig{},
		DatabaseProvider: databaseProvider,
		SecretProvider:   secretProvider,
		SpecLoader:       specLoader,
		StatusManager:    statusManager,
	}

	if configureModules == nil {
		configureModules = api.DefaultModules
	}

	modules := configureModules(options)

	err = api.Register(ctx, router, modules, options)
	require.NoError(t, err)

	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Starting HTTP server...")
	server.Start()
	logger.Info(fmt.Sprintf("Started HTTP server on %s...", server.URL))

	ucp := &TestServer{
		BaseURL: server.URL + pathBase,
		Clients: &TestServerClients{
			QueueProvider:    queueProvider,
			SecretProvider:   secretProvider,
			DatabaseProvider: databaseProvider,
		},
		Mocks: &TestServerMocks{
			Secrets:  secretClient,
			Database: databaseClient,
		},
		Server: server,
		cancel: cancel,
		t:      t,
	}

	t.Cleanup(ucp.Close)
	return ucp
}

// StartWithETCD creates and starts a new TestServer that used an embedded ETCD instance for storage.
func StartWithETCD(t *testing.T, configureModules func(options modules.Options) []modules.Initializer) *TestServer {
	config := hosting.NewAsyncValue[etcdclient.Client]()
	etcd := data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{
		ClientConfigSink:  config,
		AssignRandomPorts: true,
		Quiet:             false,
	})

	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	stoppedChan := make(chan struct{})
	defer close(stoppedChan)
	go func() {
		// We can't pass the test logger into the etcd service because it is forbidden to log
		// using the test logger after the test finishes.
		//
		// https://github.com/golang/go/issues/40343
		//
		// If you need to see the logging output while you are testing, then comment out the next line
		// and you'll be able to see the spam from etcd.
		//
		// This is caught by the race checker and will fail your pr if you do it.
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		err := etcd.Run(ctx)
		if err != nil {
			t.Logf("error from etcd: %v", err)
		}
	}()

	databaseOptions := databaseprovider.Options{
		Provider: databaseprovider.TypeETCD,
		ETCD: databaseprovider.ETCDOptions{
			InMemory: true,
			Client:   config,
		},
	}
	secretOptions := secretprovider.SecretProviderOptions{
		Provider: secretprovider.TypeETCDSecret,
		ETCD:     databaseOptions.ETCD,
	}
	queueOptions := queueprovider.QueueProviderOptions{
		Name:     server.UCPProviderName,
		Provider: queueprovider.TypeInmemory,
		InMemory: &queueprovider.InMemoryQueueOptions{},
	}

	// Generate a random base path to ensure we're handling it correctly.
	pathBase := "/" + uuid.New().String()
	databaseProvider := databaseprovider.FromOptions(databaseOptions)
	secretProvider := secretprovider.NewSecretProvider(secretOptions)
	queueProvider := queueprovider.New(queueOptions)

	queueClient, err := queueProvider.GetClient(ctx)
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Use(servicecontext.ARMRequestCtx(pathBase, "global"))

	app := middleware.NormalizePath(router)
	server := httptest.NewUnstartedServer(app)
	server.Config.BaseContext = func(l net.Listener) context.Context {
		return ctx
	}

	// Unfortunately the server doesn't populate 'server.URL' until it is started, so we have to build it ourselves.
	address := "http://" + server.Listener.Addr().String()
	connection, err := sdk.NewDirectConnection(address + pathBase)
	require.NoError(t, err)

	databaseClient, err := databaseProvider.GetClient(ctx)
	require.NoError(t, err)

	statusManager := statusmanager.New(databaseClient, queueClient, v1.LocationGlobal)

	specLoader, err := validator.LoadSpec(ctx, "ucp", swagger.SpecFilesUCP, []string{pathBase}, "")
	require.NoError(t, err, "failed to load OpenAPI spec")

	options := modules.Options{
		Address:          address,
		PathBase:         pathBase,
		Config:           &hostoptions.UCPConfig{},
		DatabaseProvider: databaseProvider,
		SecretProvider:   secretProvider,
		SpecLoader:       specLoader,
		QueueProvider:    queueProvider,
		StatusManager:    statusManager,
	}

	if configureModules == nil {
		configureModules = api.DefaultModules
	}

	modules := configureModules(options)

	// The URL for the dynamic-rp needs to be specified in configuration, however not all of our tests
	// need to use the dynamic-rp. We can just use a placeholder value here.
	if options.Config.Routing.DefaultDownstreamEndpoint == "" {
		options.Config.Routing.DefaultDownstreamEndpoint = "http://localhost:65535"
	}

	defaultDownstream, err := url.Parse(options.Config.Routing.DefaultDownstreamEndpoint)
	require.NoError(t, err)

	registry := worker.NewControllerRegistry()
	err = backend.RegisterControllers(registry, connection, http.DefaultTransport, backend_ctrl.Options{DatabaseClient: databaseClient}, defaultDownstream)
	require.NoError(t, err)

	w := worker.New(worker.Options{}, statusManager, queueClient, registry)
	go func() {
		err = w.Start(ctx)
		require.NoError(t, err)
	}()

	err = api.Register(ctx, router, modules, options)
	require.NoError(t, err)

	logger := logr.FromContextOrDiscard(ctx)
	logger.Info("Starting HTTP server...")
	server.Start()
	logger.Info(fmt.Sprintf("Started HTTP server on %s...", server.URL))

	logger.Info("Connecting to data store...")
	client, err := config.Get(ctx)
	require.NoError(t, err, "failed to access etcd client")
	_, err = client.Cluster.MemberList(ctx)
	require.NoError(t, err, "failed to query etcd")
	logger.Info("Connected to data store")

	// TODO: start worker

	ucp := &TestServer{
		BaseURL: server.URL + pathBase,
		Clients: &TestServerClients{
			QueueProvider:    queueProvider,
			SecretProvider:   secretProvider,
			DatabaseProvider: databaseProvider,
		},
		Server:      server,
		cancel:      cancel,
		etcdService: etcd,
		etcdClient:  client,
		t:           t,
		stoppedChan: stoppedChan,
	}
	t.Cleanup(ucp.Close)
	return ucp
}

// TestResponse is return from requests made against a TestServer. Tests should use the functions defined
// on TestResponse for valiation.
type TestResponse struct {
	Raw    *http.Response
	Body   *bytes.Buffer
	Error  *v1.ErrorResponse
	t      *testing.T
	server *TestServer
}

// MakeFixtureRequest sends a request to the server using a file on disk as the payload (body). Use the fixture
// parameter to specify the path to a file.
func (ts *TestServer) MakeFixtureRequest(method string, pathAndQuery string, fixture string) *TestResponse {
	body, err := os.ReadFile(fixture)
	require.NoError(ts.t, err, "reading fixture failed")
	return ts.MakeRequest(method, pathAndQuery, body)
}

// MakeTypedRequest sends a request to the server by marshalling the provided object to JSON.
func (ts *TestServer) MakeTypedRequest(method string, pathAndQuery string, body any) *TestResponse {
	if body == nil {
		return ts.MakeRequest(method, pathAndQuery, nil)
	}

	b, err := json.Marshal(body)
	require.NoError(ts.t, err, "marshalling body failed")
	return ts.MakeRequest(method, pathAndQuery, b)
}

// MakeRequest sends a request to the server.
func (ts *TestServer) MakeRequest(method string, pathAndQuery string, body []byte) *TestResponse {
	// Prepend the base path if this is a relative URL.
	requestUrl := pathAndQuery
	parsed, err := url.Parse(pathAndQuery)
	require.NoError(ts.t, err, "parsing URL failed")
	if !parsed.IsAbs() {
		requestUrl = ts.BaseURL + pathAndQuery
	}

	client := ts.Server.Client()
	request, err := rpctest.NewHTTPRequestWithContent(context.Background(), method, requestUrl, body)
	require.NoError(ts.t, err, "creating request failed")

	ctx := rpctest.NewARMRequestContext(request)
	request = request.WithContext(ctx)

	response, err := client.Do(request)
	require.NoError(ts.t, err, "sending request failed")

	// Buffer the response so we can read multiple times.
	responseBuffer := &bytes.Buffer{}
	_, err = io.Copy(responseBuffer, response.Body)
	response.Body.Close()
	require.NoError(ts.t, err, "copying response failed")

	response.Body = io.NopCloser(responseBuffer)

	// Pretty-print response for logs.
	if len(responseBuffer.Bytes()) > 0 {
		var data any
		err = json.Unmarshal(responseBuffer.Bytes(), &data)
		require.NoError(ts.t, err, "unmarshalling response failed")

		text, err := json.MarshalIndent(&data, "", "  ")
		require.NoError(ts.t, err, "marshalling response failed")
		ts.t.Log("Response Body: \n" + string(text))
	}

	var errorResponse *v1.ErrorResponse
	if response.StatusCode >= 400 {
		// The response MUST be an arm error for a non-success status code.
		errorResponse = &v1.ErrorResponse{}
		err := json.Unmarshal(responseBuffer.Bytes(), &errorResponse)
		require.NoError(ts.t, err, "unmarshalling error response failed - THIS IS A SERIOUS BUG. ALL ERROR RESPONSES MUST USE THE STANDARD FORMAT")
	}

	return &TestResponse{Raw: response, Body: responseBuffer, Error: errorResponse, server: ts, t: ts.t}
}

// EqualsErrorCode compares a TestResponse against an expected status code and error code. EqualsErrorCode assumes the response
// uses the ARM error format (required for our APIs).
func (tr *TestResponse) EqualsErrorCode(statusCode int, code string) {
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
	require.NotNil(tr.t, tr.Error, "expected an error but actual response did not contain one")
	require.Equal(tr.t, code, tr.Error.Error.Code, "actual error code was different from expected")
}

// EqualsFixture compares a TestResponse against an expected status code and body payload. Use the fixture parameter to specify
// the path to a file.
func (tr *TestResponse) EqualsFixture(statusCode int, fixture string) {
	body, err := os.ReadFile(fixture)
	require.NoError(tr.t, err, "reading fixture failed")
	tr.EqualsResponse(statusCode, body)
}

// EqualsStatusCode compares a TestResponse against an expected status code (ingnores the body payload).
func (tr *TestResponse) EqualsStatusCode(statusCode int) {
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsFixture compares a TestResponse against an expected status code and body payload.
func (tr *TestResponse) EqualsResponse(statusCode int, body []byte) {
	if len(body) == 0 {
		require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
		require.Empty(tr.t, tr.Body.Bytes(), "expected an empty response but actual response had a body")
		return
	}

	var expected map[string]any
	err := json.Unmarshal(body, &expected)
	require.NoError(tr.t, err, "unmarshalling expected response failed")

	var actual map[string]any
	err = json.Unmarshal(tr.Body.Bytes(), &actual)

	tr.removeSystemData(actual)

	require.NoError(tr.t, err, "unmarshalling actual response failed")
	require.EqualValues(tr.t, expected, actual, "response body did not match expected")
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsValue compares a TestResponse against an expected status code and an response body.
//
// If the systemData propert is present in the response, it will be removed.
func (tr *TestResponse) EqualsValue(statusCode int, expected any) {
	var actual map[string]any
	err := json.Unmarshal(tr.Body.Bytes(), &actual)
	require.NoError(tr.t, err, "unmarshalling actual response failed")

	// Convert expected input to map[string]any to compare with actual response.
	expectedBytes, err := json.Marshal(expected)
	require.NoError(tr.t, err, "marshalling expected response failed")

	var expectedMap map[string]any
	err = json.Unmarshal(expectedBytes, &expectedMap)
	require.NoError(tr.t, err, "unmarshalling expected response failed")

	tr.removeSystemData(expectedMap)
	tr.removeSystemData(actual)

	require.EqualValues(tr.t, expectedMap, actual, "response body did not match expected")
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
}

// EqualsEmptyList compares a TestResponse against an expected status code and an empty resource list.
func (tr *TestResponse) EqualsEmptyList() {
	expected := map[string]any{
		"value": []any{},
	}

	var actual map[string]any
	err := json.Unmarshal(tr.Body.Bytes(), &actual)

	tr.removeSystemData(actual)

	require.NoError(tr.t, err, "unmarshalling actual response failed")
	require.EqualValues(tr.t, expected, actual, "response body did not match expected")
	require.Equal(tr.t, http.StatusOK, tr.Raw.StatusCode, "status code did not match expected")
}

func (tr *TestResponse) ReadAs(obj any) {
	tr.t.Helper()

	decoder := json.NewDecoder(tr.Body)
	decoder.DisallowUnknownFields()

	err := decoder.Decode(obj)
	require.NoError(tr.t, err, "unmarshalling expected response failed")
}

func (tr *TestResponse) WaitForOperationComplete(timeout *time.Duration) *TestResponse {
	if tr.Raw.StatusCode != http.StatusCreated && tr.Raw.StatusCode != http.StatusAccepted {
		// Response is already terminal.
		return tr
	}

	if timeout == nil {
		x := 30 * time.Second
		timeout = &x
	}

	timer := time.After(*timeout)
	poller := time.NewTicker(1 * time.Second)
	defer poller.Stop()
	for {
		select {
		case <-timer:
			tr.t.Fatalf("timed out waiting for operation to complete")
			return nil // unreachable
		case <-poller.C:
			// The Location header should give us the operation status URL.
			response := tr.server.MakeRequest(http.MethodGet, tr.Raw.Header.Get("Azure-AsyncOperation"), nil)
			// To determine if the response is terminal we need to read the provisioning state field.

			operationStatus := v1.AsyncOperationStatus{}
			response.ReadAs(&operationStatus)
			if operationStatus.Status.IsTerminal() {
				// Response is terminal.
				return response
			}

			continue
		}
	}
}

func (tr *TestResponse) removeSystemData(responseBody map[string]any) {
	// Delete systemData property if found, it's not stable so we don't include it in baselines.
	_, ok := responseBody["systemData"]
	if ok {
		delete(responseBody, "systemData")
		return
	}

	value, ok := responseBody["value"]
	if !ok {
		return
	}

	valueSlice, ok := value.([]any)
	if !ok {
		return
	}

	for _, v := range valueSlice {
		if vMap, ok := v.(map[string]any); ok {
			tr.removeSystemData(vMap)
		}
	}
}
