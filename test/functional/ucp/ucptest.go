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
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	cli "github.com/radius-project/radius/pkg/cli"
	"github.com/radius-project/radius/pkg/cli/connections"
	"github.com/radius-project/radius/pkg/cli/kubernetes"
	"github.com/radius-project/radius/pkg/sdk"
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

type TestRunMethod func(t *testing.T, url string, roundtripper http.RoundTripper)

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
}

type TestStep struct {
}

// NewUCPTest creates a new UCPTest instance with the given name and run method.
func NewUCPTest(t *testing.T, name string, runMethod TestRunMethod) UCPTest {
	return UCPTest{
		Options:     test.NewTestOptions(t),
		Name:        name,
		Description: name,
		RunMethod:   runMethod,
	}
}

func (ucptest UCPTest) Test(t *testing.T) {
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

	config, err := kubernetes.NewCLIClientConfig("")
	require.NoError(t, err, "failed to read kubeconfig")

	connection, err := sdk.NewKubernetesConnectionFromConfig(config)
	require.NoError(t, err, "failed to create kubernetes connection")

	// Transport will be nil for some default cases as http.Client does not require it to be set.
	// Since the tests call the transport directly then just pass in the default.
	transport := connection.Client().Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	ucptest.RunMethod(t, connection.Endpoint(), transport)

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

		exists := credentialExists(t, credential)
		if !exists {
			t.Skip(message)
		}
	}
}

func credentialExists(t *testing.T, credential string) bool {
	ctx := testcontext.New(t)

	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	workspace, err := cli.GetWorkspace(config, "")
	require.NoError(t, err, "failed to read default workspace")
	require.NotNil(t, workspace, "default workspace is not set")

	t.Logf("Loaded workspace: %s (%s)", workspace.Name, workspace.FmtConnection())

	credentialsClient, err := connections.DefaultFactory.CreateCredentialManagementClient(ctx, *workspace)
	require.NoError(t, err, "failed to create credentials client")
	cred, err := credentialsClient.Get(ctx, credential)
	require.NoError(t, err, "failed to get credentials")

	return cred.CloudProviderStatus.Enabled
}
