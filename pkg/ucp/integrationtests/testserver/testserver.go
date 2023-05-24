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
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	armrpc_controller "github.com/project-radius/radius/pkg/armrpc/frontend/controller"
	"github.com/project-radius/radius/pkg/armrpc/servicecontext"
	"github.com/project-radius/radius/pkg/middleware"
	"github.com/project-radius/radius/pkg/ucp/data"
	"github.com/project-radius/radius/pkg/ucp/dataprovider"
	"github.com/project-radius/radius/pkg/ucp/frontend/api"
	"github.com/project-radius/radius/pkg/ucp/frontend/controller"
	"github.com/project-radius/radius/pkg/ucp/hosting"
	secretprovider "github.com/project-radius/radius/pkg/ucp/secret/provider"
	"github.com/project-radius/radius/pkg/ucp/util/testcontext"
	"github.com/project-radius/radius/test/testutil"
	"github.com/stretchr/testify/require"
	etcdclient "go.etcd.io/etcd/client/v3"
)

// TestServer can run a UCP server using the Go httptest package. It provides access to an isolated ETCD instances for storage
// of resources and secrets.
//
// Do not create a TestServer directly, use Start instead.
type TestServer struct {
	BaseURL     string
	cancel      context.CancelFunc
	etcdService *data.EmbeddedETCDService
	server      *httptest.Server
	client      *etcdclient.Client
	t           *testing.T
	stoppedChan <-chan struct{}
}

// Client provides access to an http.Client that can be used to send requests. Most tests should use the functionality
// like MakeRequest instead of testing the client directly.
func (ts *TestServer) Client() *http.Client {
	return ts.server.Client()
}

// Close shuts down the server and will block until shutdown completes.
func (ts *TestServer) Close() {
	// We're being picking about resource cleanup here, because unless we are picky we hit scalability
	// problems in tests pretty quickly.

	ts.cancel() // Start ETCD shutdown

	ts.server.Close() // Stop HTTP server
	ts.client.Close() // Stop ETCD Client

	<-ts.stoppedChan // ETCD stopped
}

// Start creates and starts a new TestServer.
func Start(t *testing.T) *TestServer {
	config := hosting.NewAsyncValue[etcdclient.Client]()
	etcd := data.NewEmbeddedETCDService(data.EmbeddedETCDServiceOptions{
		ClientConfigSink:  config,
		AssignRandomPorts: true,
		Quiet:             false,
	})

	ctx, cancel := testcontext.New(t)

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
		ctx := context.Background()
		err := etcd.Run(ctx)
		if err != nil {
			t.Logf("error from etcd: %v", err)
		}
	}()

	storageOptions := dataprovider.StorageProviderOptions{
		Provider: dataprovider.TypeETCD,
		ETCD: dataprovider.ETCDOptions{
			InMemory: true,
			Client:   config,
		},
	}
	secretOptions := secretprovider.SecretProviderOptions{
		Provider: secretprovider.TypeETCDSecret,
		ETCD:     storageOptions.ETCD,
	}
	secretProvider := secretprovider.NewSecretProvider(secretOptions)
	secretClient, err := secretProvider.GetClient(ctx)
	require.NoError(t, err)

	// Generate a random base path to ensure we're handling it correctly.
	basePath := "/" + uuid.New().String()

	// TODO: remove this to align on design with the RPs
	storageClient, err := dataprovider.NewStorageProvider(storageOptions).GetStorageClient(ctx, "ucp")
	require.NoError(t, err)

	router := mux.NewRouter()

	router.Use(servicecontext.ARMRequestCtx(basePath, "global"))

	app := http.Handler(router)
	app = middleware.NormalizePath(app)
	server := httptest.NewUnstartedServer(app)
	server.Config.BaseContext = func(l net.Listener) context.Context {
		return ctx
	}
	err = api.Register(ctx, router, controller.Options{
		Options: armrpc_controller.Options{
			DataProvider:  dataprovider.NewStorageProvider(storageOptions),
			StorageClient: storageClient,
		},
		// TODO: we're doing lots of cleanup on controller.Options that will lead
		// to small changes here.
		Address:  server.URL,
		BasePath: basePath,

		// TODO: remove this to align on design with the RPs
		SecretClient: secretClient,
	},
	)
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

	return &TestServer{
		BaseURL:     server.URL + basePath,
		cancel:      cancel,
		server:      server,
		etcdService: etcd,
		client:      client,
		t:           t,
		stoppedChan: stoppedChan,
	}
}

// TestResponse is return from requests made against a TestServer. Tests should use the functions defined
// on TestResponse for valiation.
type TestResponse struct {
	Raw   *http.Response
	Body  *bytes.Buffer
	Error *v1.ErrorResponse
	t     *testing.T
}

// MakeFixtureRequest sends a request to the server using a file on disk as the payload (body). Use the fixture
// parameter to specify the path to a file.
func (ts *TestServer) MakeFixtureRequest(method string, pathAndQuery string, fixture string) *TestResponse {
	body, err := os.ReadFile(fixture)
	require.NoError(ts.t, err, "reading fixture failed")
	return ts.MakeRequest(method, pathAndQuery, body)
}

// MakeRequest sends a request to the server.
func (ts *TestServer) MakeRequest(method string, pathAndQuery string, body []byte) *TestResponse {
	client := ts.server.Client()
	request, err := testutil.GetARMTestHTTPRequestFromURL(context.Background(), method, ts.BaseURL+pathAndQuery, body)
	require.NoError(ts.t, err, "creating request failed")

	ctx := testutil.ARMTestContextFromRequest(request)
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

	return &TestResponse{Raw: response, Body: responseBuffer, Error: errorResponse, t: ts.t}
}

// EqualsErrorCode compares a TestResponse against an expected status code and error code. EqualsErrorCode assumes the response
// uses the ARM error format (required for our APIs).
func (tr *TestResponse) EqualsErrorCode(statusCode int, code string) {
	require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
	require.NotNil(tr.t, tr.Error, "expected an error but actual response did not contain one")
	require.Equal(tr.t, code, tr.Error.Error.Code, "actual error code was different from expected")
}

// EqualsFixture compares a TestResponse against an expected status code and body payload. Use the fixture paramter to specify
// the path to a file.
func (tr *TestResponse) EqualsFixture(statusCode int, fixture string) {
	body, err := os.ReadFile(fixture)
	require.NoError(tr.t, err, "reading fixture failed")
	tr.EqualsResponse(statusCode, body)
}

// EqualsFixture compares a TestResponse against an expected status code and body payload.
func (tr *TestResponse) EqualsResponse(statusCode int, body []byte) {
	if len(body) == 0 {
		require.Equal(tr.t, statusCode, tr.Raw.StatusCode, "status code did not match expected")
		require.Empty(tr.t, tr.Body.Bytes(), "expected an empty response but actual response had a body")
		return
	}

	var expected any
	err := json.Unmarshal(body, &expected)
	require.NoError(tr.t, err, "unmarshalling expected response failed")

	var actual any
	err = json.Unmarshal(tr.Body.Bytes(), &actual)
	require.NoError(tr.t, err, "unmarshalling actual response failed")
	require.EqualValues(tr.t, expected, actual, "response body did not match expected")
}
