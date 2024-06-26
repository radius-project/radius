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

package ucp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/clients_new/generated"
	"github.com/radius-project/radius/pkg/sdk"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
	awsMessage             = "This test requires AWS. Please configure the test environment to include an AWS provider."
	azureMessage           = "This test requires Azure. Please configure the test environment to include an Azure provider."
)

var radiusControllerLogSync sync.Once

type TestRunMethod func(t *testing.T, test *UCPTest)

// RequiredFeature is used to specify an optional feature that is required
// for the test to run.
type RequiredFeature string

const (
	// FeatureAWS should be used with required features to indicate a test dependency on AWS cloud provider.
	FeatureAWS RequiredFeature = "AWS"

	// FeatureAzure should be used with required features to indicate a test dependency on Azure cloud provider.
	FeatureAzure RequiredFeature = "Azure"
)

type UCPTest struct {
	Options     test.TestOptions
	Name        string
	Description string
	RunMethod   TestRunMethod

	// RequiredFeatures is a list of features that are required for the test to run.
	RequiredFeatures []RequiredFeature

	// Connection is the connection to UCP.
	Connection sdk.Connection

	// URL is the base URL of UCP for the test.
	URL string

	// Transport is the HTTP transport to use for the test.
	Transport http.RoundTripper
}

// NewUCPTest creates a new UCPTest instance with the given name and run method.
func NewUCPTest(t *testing.T, name string, runMethod TestRunMethod) *UCPTest {
	return &UCPTest{
		Options:     test.NewTestOptions(t),
		Name:        name,
		Description: name,
		RunMethod:   runMethod,
	}
}

func (ucptest *UCPTest) Test(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)

	ucptest.CheckRequiredFeatures(ctx, t)

	t.Cleanup(cancel)

	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs/ucptest"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		_, err := validation.SaveContainerLogs(ctx, ucptest.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")
	require.NotNil(t, workspace, "default workspace is not set")

	t.Logf("Loaded workspace: %s (%s)", workspace.Name, workspace.FmtConnection())

	connection, err := workspace.Connect()
	require.NoError(t, err, "failed to connect to workspace")

	// Store the connection for later
	ucptest.Connection = connection
	ucptest.URL = connection.Endpoint()

	// Transport will be nil for some default cases as http.Client does not require it to be set.
	// Since the tests call the transport directly then just pass in the default.
	ucptest.Transport = connection.Client().Transport
	if ucptest.Transport == nil {
		ucptest.Transport = http.DefaultTransport
	}

	ucptest.RunMethod(t, ucptest)

}

// NewUCPRequest creates an HTTP request with the given method, URL and body, and adds a Content-Type header to it,
// returning the request or an error if one occurs.
func NewUCPRequest(method string, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	return req, nil
}

// CheckRequiredFeatures checks the test environment for the features that the test requires and skips the test if not, otherwise
// returns an error if there is an issue.
func (ct UCPTest) CheckRequiredFeatures(ctx context.Context, t *testing.T) {
	for _, feature := range ct.RequiredFeatures {
		var credential, message string
		switch feature {
		case FeatureAWS:
			message = awsMessage
			credential = "aws"
		case FeatureAzure:
			message = azureMessage
			credential = "azure"
		default:
			panic(fmt.Sprintf("unsupported feature: %s", feature))
		}

		exists := validation.AssertCredentialExists(t, credential)
		if !exists {
			t.Skip(message)
		}
	}
}

func (u *UCPTest) CreateGenericClient(t *testing.T, scope string, resourceType string) *generated.GenericResourcesClient {
	if u.Connection == nil {
		t.Fatal("CreateGenericClient should be called after the test has started.")
	}

	client, err := generated.NewGenericResourcesClient(scope, resourceType, &aztoken.AnonymousCredential{}, sdk.NewClientOptions(u.Connection))
	require.NoError(t, err)
	return client
}

func (u *UCPTest) CreateResource(t *testing.T, id string, resource any) {
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)

	client := u.CreateGenericClient(t, parsed.RootScope(), parsed.Type())

	b, err := json.Marshal(resource)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()

	payload := generated.GenericResource{}
	err = decoder.Decode(&payload)
	require.NoError(t, err)

	ctx := testcontext.New(t)
	poller, err := client.BeginCreateOrUpdate(ctx, parsed.Name(), payload, nil)
	require.NoError(t, err)
	t.Logf("Creating resource %s", id)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	t.Logf("Resource %s created", id)

}

func (u *UCPTest) DeleteResource(t *testing.T, id string) {
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)

	client := u.CreateGenericClient(t, parsed.RootScope(), parsed.Type())

	ctx := testcontext.New(t)
	poller, err := client.BeginDelete(ctx, parsed.Name(), nil)
	require.NoError(t, err)
	t.Logf("Deleting resource %s", id)

	_, err = poller.PollUntilDone(ctx, nil)
	require.NoError(t, err)
	t.Logf("Resource %s deleted", id)
}

func (u *UCPTest) GetResource(t *testing.T, id string, resource any) {
	parsed, err := resources.ParseResource(id)
	require.NoError(t, err)

	client := u.CreateGenericClient(t, parsed.RootScope(), parsed.Type())

	ctx := testcontext.New(t)
	response, err := client.Get(ctx, parsed.Name(), nil)
	require.NoError(t, err)

	b, err := json.Marshal(response.GenericResource)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&resource)
	require.NoError(t, err)
}
