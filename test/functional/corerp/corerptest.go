// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package corerp

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"

	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
	APIVersion             = "2022-03-15-privatepreview"
)

type TestStep struct {
	Executor  step.Executor
	Resources []validation.Resource
}

type CoreRPTest struct {
	Options          CoreRPTestOptions
	Name             string
	Description      string
	Steps            []TestStep
	PostDeleteVerify func(ctx context.Context, t *testing.T, ct CoreRPTest)
}

type TestOptions struct {
	test.TestOptions
	DiscoveryClient discovery.DiscoveryInterface
}

func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{TestOptions: test.NewTestOptions(t)}
}

func NewCoreRPTest(t *testing.T, name string, steps []TestStep, initialResources ...unstructured.Unstructured) CoreRPTest {
	return CoreRPTest{
		Options:     NewCoreRPTestOptions(t),
		Name:        name,
		Description: name,
		Steps:       steps,
	}
}

func (ct CoreRPTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	// TODO: not sure if this is true for corerp tests
	// t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		err := validation.SaveLogsForController(ctx, ct.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	err := validation.SaveLogsForApplication(ctx, ct.Options.K8sClient, ct.Name, logPrefix+"/"+ct.Name, ct.Name)
	if err != nil {
		t.Errorf("failed to capture logs from radius pods %v", err)
	}

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	require.GreaterOrEqual(t, len(ct.Steps), 1, "at least one step is required")

	success := true
	for i, step := range ct.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(ct.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, ct.Options.TestOptions)
			t.Logf("finished running step %d of %d: %s", i, len(ct.Steps), step.Executor.GetDescription())

			// Validate resources
			validation.ValidateCoreRPResources(ctx, t, step.Resources, ct.Options.ManagementClient)
		})
	}

	// Clean up resources
	// TODO: re-enable cleanup of application (and environments)
}
