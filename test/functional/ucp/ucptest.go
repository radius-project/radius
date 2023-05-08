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
	"io"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/pkg/sdk"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
)

const ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"

var radiusControllerLogSync sync.Once

type TestRunMethod func(t *testing.T, url string, roundtripper http.RoundTripper)

type UCPTest struct {
	Options     test.TestOptions
	Name        string
	Description string
	RunMethod   TestRunMethod
}

type TestStep struct {
}

func NewUCPTest(t *testing.T, name string, runMethod TestRunMethod) UCPTest {
	return UCPTest{
		Options:     test.NewTestOptions(t),
		Name:        name,
		Description: name,
		RunMethod:   runMethod,
	}
}

func (ucptest UCPTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

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

func NewUCPRequest(method string, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")

	return req, nil
}
