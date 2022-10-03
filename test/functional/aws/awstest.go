// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package awstest

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	awsclient "github.com/project-radius/radius/pkg/ucp/aws"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
)

const ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"

type TestStep struct {
	Executor     step.Executor
	AWSResources *validation.AWSResourceSet
}

type AWSTest struct {
	Options          AWSTestOptions
	Name             string
	Description      string
	InitialResources []unstructured.Unstructured
	Steps            []TestStep
	PostDeleteVerify func(ctx context.Context, t *testing.T, at AWSTest)
}
type TestOptions struct {
	test.TestOptions
	DiscoveryClient discovery.DiscoveryInterface
}

func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{TestOptions: test.NewTestOptions(t)}
}

func NewAWSTest(t *testing.T, name string, steps []TestStep) AWSTest {
	return AWSTest{
		Options:     NewAWSTestOptions(t),
		Name:        name,
		Description: name,
		Steps:       steps,
	}
}

type AWSTestOptions struct {
	test.TestOptions
}

func NewAWSTestOptions(t *testing.T) AWSTestOptions {
	return AWSTestOptions{
		TestOptions: test.NewTestOptions(t),
	}
}

var radiusControllerLogSync sync.Once

func (at AWSTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

	cfg, err := config.LoadDefaultConfig(ctx)
	require.NoError(t, err)

	var client awsclient.AWSClient = cloudcontrol.NewFromConfig(cfg)

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// Each of our tests are isolated, so they can run in parallel.
	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs/awstest"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		err := validation.SaveLogsForController(ctx, at.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}

		// Getting logs from all pods in the default namespace as well, which is where all app pods run for calls to rad deploy
		err = validation.SaveLogsForController(ctx, at.Options.K8sClient, "default", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	require.GreaterOrEqual(t, len(at.Steps), 1, "at least one step is required")

	success := true
	for i, step := range at.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options.TestOptions)
			t.Logf("finished running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())

			if step.AWSResources == nil || len(step.AWSResources.Resources) == 0 {
				require.Fail(t, "no resource set was specified.")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())
				validation.ValidateAWSResources(ctx, t, step.AWSResources, client)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

		})
	}

	t.Logf("beginning cleanup phase of %s", at.Description)

	// Cleanup code here will run regardless of pass/fail of subtests
	for _, step := range at.Steps {
		for _, resource := range step.AWSResources.Resources {
			t.Logf("deleting %s", resource.Name)
			validation.DeleteAWSResource(ctx, t, &resource, client)
			require.NoErrorf(t, err, "failed to delete %s", resource.Name)
			t.Logf("finished deleting %s", at.Description)
		}
	}
	t.Logf("finished cleanup phase of %s", at.Description)
}
