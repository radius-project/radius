// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package corerp

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/restmapper"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	memory "k8s.io/client-go/discovery/cached"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
	APIVersion             = "2022-03-15-privatepreview"
	TestNamespace          = "kind-radius"
)

type TestStep struct {
	Executor               step.Executor
	CoreRPResources        *validation.CoreRPResourceSet
	K8sOutputResources     []unstructured.Unstructured
	K8sObjects             *validation.K8sObjectSet
	PostStepVerify         func(ctx context.Context, t *testing.T, ct CoreRPTest)
	SkipResourceValidation bool
	SkipObjectValidation   bool
}

type CoreRPTest struct {
	Options          CoreRPTestOptions
	Name             string
	Description      string
	InitialResources []unstructured.Unstructured
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

func (ct CoreRPTest) CreateInitialResources(ctx context.Context) error {
	err := kubernetes.EnsureNamespace(ctx, ct.Options.K8sClient, ct.Name)
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", ct.Name, err)
	}

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(ct.Options.K8sClient.Discovery()))
	for _, r := range ct.InitialResources {
		mapping, err := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if err != nil {
			return fmt.Errorf("unknown kind %q: %w", r.GroupVersionKind().String(), err)
		}
		if mapping.Scope == meta.RESTScopeNamespace {
			_, err = ct.Options.DynamicClient.Resource(mapping.Resource).
				Namespace(ct.Name).
				Create(ctx, &r, v1.CreateOptions{})
		} else {
			_, err = ct.Options.DynamicClient.Resource(mapping.Resource).
				Create(ctx, &r, v1.CreateOptions{})
		}
		if err != nil {
			return fmt.Errorf("failed to create %q resource %#v:  %w", mapping.Resource.String(), r, err)
		}
	}
	return nil
}

func (ct CoreRPTest) CleanUpExtensionResources(resources []unstructured.Unstructured) {
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(ct.Options.K8sClient.Discovery()))
	for _, r := range resources {
		mapping, _ := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if mapping.Scope == meta.RESTScopeNamespace {
			_ = ct.Options.DynamicClient.Resource(mapping.Resource).
				Namespace(r.GetNamespace()).
				Delete(context.TODO(), r.GetName(), v1.DeleteOptions{})
		} else {
			_ = ct.Options.DynamicClient.Resource(mapping.Resource).
				Delete(context.TODO(), r.GetName(), v1.DeleteOptions{})
		}
	}
}

func (ct CoreRPTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

	cli := radcli.NewCLI(t, ct.Options.ConfigFilePath)

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated, so they can run in parallel.
	t.Parallel()

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
			defer ct.CleanUpExtensionResources(step.K8sOutputResources)
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(ct.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, ct.Options.TestOptions)
			t.Logf("finished running step %d of %d: %s", i, len(ct.Steps), step.Executor.GetDescription())

			if step.SkipResourceValidation {
				t.Logf("skipping validation of resources...")
			} else if step.CoreRPResources == nil || len(step.CoreRPResources.Resources) == 0 {
				require.Fail(t, "no resource set was specified and SkipResourceValidation == false, either specify a resource set or set SkipResourceValidation = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())
				validation.ValidateCoreRPResources(ctx, t, step.CoreRPResources, ct.Options.ManagementClient)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

			if step.SkipObjectValidation {
				t.Logf("skipping validation of objects...")
			} else if step.K8sObjects == nil && len(step.K8sOutputResources) == 0 {
				require.Fail(t, "no objects specified and SkipObjectValidation == false, either specify an object set or set SkipObjectValidation = true ")
			} else {
				if step.K8sObjects != nil {
					t.Logf("validating creation of objects for %s", step.Executor.GetDescription())
					validation.ValidateObjectsRunning(ctx, t, ct.Options.K8sClient, ct.Options.DynamicClient, *step.K8sObjects)
					t.Logf("finished validating creation of objects for %s", step.Executor.GetDescription())
				}
			}

			// Custom verification is expected to use `t` to trigger its own assertions
			if step.PostStepVerify != nil {
				t.Logf("running post-deploy verification for %s", step.Executor.GetDescription())
				step.PostStepVerify(ctx, t, ct)
				t.Logf("finished post-deploy verification for %s", step.Executor.GetDescription())
			}
		})
	}

	t.Logf("beginning cleanup phase of %s", ct.Description)

	// Cleanup code here will run regardless of pass/fail of subtests
	for _, step := range ct.Steps {
		for _, resource := range step.CoreRPResources.Resources {
			t.Logf("deleting %s", resource.Name)
			err := validation.DeleteCoreRPResource(ctx, t, cli, ct.Options.ManagementClient, resource)
			require.NoErrorf(t, err, "failed to delete %s", resource.Name)
			t.Logf("finished deleting %s", ct.Description)

			if step.SkipObjectValidation {
				t.Logf("skipping validation of deletion of pods...")
			} else {
				t.Logf("validating deletion of pods for %s", ct.Description)
				validation.ValidateNoPodsInApplication(ctx, t, ct.Options.K8sClient, TestNamespace, ct.Name)
				t.Logf("finished validation of deletion of pods for %s", ct.Description)
			}
		}
	}

	// Custom verification is expected to use `t` to trigger its own assertions
	if ct.PostDeleteVerify != nil {
		t.Logf("running post-delete verification for %s", ct.Description)
		ct.PostDeleteVerify(ctx, t, ct)
		t.Logf("finished post-delete verification for %s", ct.Description)
	}

	t.Logf("finished cleanup phase of %s", ct.Description)
}
