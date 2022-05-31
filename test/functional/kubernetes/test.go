// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/restmapper"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
)

type TestStep struct {
	Executor               step.Executor
	RadiusResources        *validation.ResourceSet
	K8sOutputResources     []unstructured.Unstructured
	K8sObjects             *validation.K8sObjectSet
	PostStepVerify         func(ctx context.Context, t *testing.T, at ApplicationTest)
	SkipOutputResources    bool
	SkipResourceValidation bool
}

type ApplicationTest struct {
	Options          TestOptions
	Application      string
	Description      string
	InitialResources []unstructured.Unstructured
	Steps            []TestStep
	PostDeleteVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
}

type TestOptions struct {
	test.TestOptions
	DiscoveryClient discovery.DiscoveryInterface
}

func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{TestOptions: test.NewTestOptions(t)}
}

func (at ApplicationTest) CollectAllNamespaces() []string {
	all := map[string]bool{}
	for _, step := range at.Steps {
		if step.K8sObjects != nil {
			for ns := range step.K8sObjects.Namespaces {
				all[ns] = true
			}
		}
	}

	results := []string{}
	for ns := range all {
		results = append(results, ns)
	}

	return results
}

func NewApplicationTest(t *testing.T, application string, steps []TestStep, initialResources ...unstructured.Unstructured) ApplicationTest {
	return ApplicationTest{
		Options:          NewTestOptions(t),
		Application:      application,
		Description:      application,
		InitialResources: initialResources,
		Steps:            steps,
	}
}

func (at ApplicationTest) CreateInitialResources(ctx context.Context) error {
	err := kubernetes.EnsureNamespace(ctx, at.Options.K8sClient, at.Application)
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", at.Application, err)
	}

	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(at.Options.K8sClient.Discovery()))
	for _, r := range at.InitialResources {
		mapping, err := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if err != nil {
			return fmt.Errorf("unknown kind %q: %w", r.GroupVersionKind().String(), err)
		}
		if mapping.Scope == meta.RESTScopeNamespace {
			_, err = at.Options.DynamicClient.Resource(mapping.Resource).
				Namespace(at.Application).
				Create(ctx, &r, v1.CreateOptions{})
		} else {
			_, err = at.Options.DynamicClient.Resource(mapping.Resource).
				Create(ctx, &r, v1.CreateOptions{})
		}
		if err != nil {
			return fmt.Errorf("failed to create %q resource %#v:  %w", mapping.Resource.String(), r, err)
		}
	}
	return nil
}

func (at ApplicationTest) CleanUpExtensionResources(resources []unstructured.Unstructured) {
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(at.Options.K8sClient.Discovery()))
	for _, r := range resources {
		mapping, _ := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if mapping.Scope == meta.RESTScopeNamespace {
			_ = at.Options.DynamicClient.Resource(mapping.Resource).
				Namespace(r.GetNamespace()).
				Delete(context.TODO(), r.GetName(), v1.DeleteOptions{})
		} else {
			_ = at.Options.DynamicClient.Resource(mapping.Resource).
				Delete(context.TODO(), r.GetName(), v1.DeleteOptions{})
		}
	}
}

func (at ApplicationTest) Test(t *testing.T) {
	ctx, cancel := test.GetContext(t)
	defer cancel()

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		err := validation.SaveLogsForController(ctx, at.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	err := validation.SaveLogsForApplication(ctx, at.Options.K8sClient, at.Application, logPrefix+"/"+at.Application, at.Application)
	if err != nil {
		t.Errorf("failed to capture logs from radius pods %v", err)
	}

	cli := radcli.NewCLI(t, at.Options.ConfigFilePath)

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	require.GreaterOrEqual(t, len(at.Steps), 1, "at least one step is required")
	defer at.CleanUpExtensionResources(at.InitialResources)
	err = at.CreateInitialResources(ctx)
	require.NoError(t, err, "failed to create initial resources")
	success := true
	for i, step := range at.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			defer at.CleanUpExtensionResources(step.K8sOutputResources)
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options.TestOptions)
			t.Logf("finished running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())

			if step.RadiusResources == nil && step.SkipOutputResources {
				t.Logf("skipping validation of resources...")
			} else if step.RadiusResources == nil {
				require.Fail(t, "no resource set was specified and SkipOutputResources == false, either specify a resource set or set SkipOutputResources = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())

				// TODO: create k8s client for validating output resources
				// https://github.com/project-radius/radius/issues/778
				// validation.ValidateOutputResources(t, at.Options.ARMConnection, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, *step.RadiusResources)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

			if step.SkipResourceValidation {
				t.Logf("skipping validation of resources...")
			} else if step.K8sObjects == nil && len(step.K8sOutputResources) == 0 {
				require.Fail(t, "no resources specified and SkipResourceValidation == false, either specify a resource set or set SkipResourceValidation = true ")
			} else {
				if step.K8sObjects != nil {
					t.Logf("validating creation of objects for %s", step.Executor.GetDescription())
					validation.ValidateObjectsRunning(ctx, t, at.Options.K8sClient, at.Options.DynamicClient, *step.K8sObjects)
					t.Logf("finished creation of validating objects for %s", step.Executor.GetDescription())
				}
			}

			// Custom verification is expected to use `t` to trigger its own assertions
			if step.PostStepVerify != nil {
				t.Logf("running post-deploy verification for %s", step.Executor.GetDescription())
				step.PostStepVerify(ctx, t, at)
				t.Logf("finished post-deploy verification for %s", step.Executor.GetDescription())
			}
		})
	}

	t.Logf("beginning cleanup phase of %s", at.Description)

	// We run the validation code based on the final step
	last := at.Steps[len(at.Steps)-1]

	// Cleanup code here will run regardless of pass/fail of subtests
	t.Logf("deleting %s", at.Description)
	err = cli.ApplicationDelete(ctx, at.Application)
	require.NoErrorf(t, err, "failed to delete %s", at.Description)
	t.Logf("finished deleting %s", at.Description)

	if last.SkipResourceValidation {
		t.Logf("skipping validation of pods...")
	} else {
		t.Logf("validating deletion of pods for %s", at.Description)
		for _, ns := range at.CollectAllNamespaces() {
			validation.ValidateNoPodsInApplication(ctx, t, at.Options.K8sClient, ns, at.Application)
		}
		t.Logf("finished deletion of pods for %s", at.Description)
	}

	// Custom verification is expected to use `t` to trigger its own assertions
	if at.PostDeleteVerify != nil {
		t.Logf("running post-delete verification for %s", at.Description)
		at.PostDeleteVerify(ctx, t, at)
		t.Logf("finished post-delete verification for %s", at.Description)
	}

	t.Logf("finished cleanup phase of %s", at.Description)
}
