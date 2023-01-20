// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package ucp

import (
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
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

	url, roundTripper, err := kubernetes.GetBaseUrlAndRoundTripperForDeploymentEngine("", "")
	require.NoError(t, err, "")

	ucptest.RunMethod(t, url, roundTripper)

}
