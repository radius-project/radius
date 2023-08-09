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

package shared

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/project-radius/radius/pkg/cli/kubernetes"
	"github.com/project-radius/radius/test"
	"github.com/project-radius/radius/test/radcli"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/testcontext"
	"github.com/project-radius/radius/test/validation"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
	APIVersion             = "2022-03-15-privatepreview"
	TestNamespace          = "kind-radius"
	AWSDeletionRetryLimit  = 5

	// Used to check required features
	daprComponentCRD         = "components.dapr.io"
	daprFeatureMessage       = "This test requires Dapr installed in your Kubernetes cluster. Please install Dapr by following the instructions at https://docs.dapr.io/operations/hosting/kubernetes/kubernetes-deploy/."
	secretProviderClassesCRD = "secretproviderclasses.secrets-store.csi.x-k8s.io"
	csiDriverMessage         = "This test requires secret store CSI driver. Please install it by following https://secrets-store-csi-driver.sigs.k8s.io/."
)

// RequiredFeature is used to specify an optional feature that is required
// for the test to run.
type RequiredFeature string

const (
	// FeatureDapr should used with required features to indicate a test dependency
	// on Dapr.
	FeatureDapr RequiredFeature = "Dapr"

	// FeatureCSIDriver should be used with required features to indicate a test dependency
	// on the CSI driver.
	FeatureCSIDriver RequiredFeature = "CSIDriver"
)

type TestStep struct {
	Executor                               step.Executor
	RPResources                            *validation.RPResourceSet
	K8sOutputResources                     []unstructured.Unstructured
	K8sObjects                             *validation.K8sObjectSet
	AWSResources                           *validation.AWSResourceSet
	PostStepVerify                         func(ctx context.Context, t *testing.T, ct RPTest)
	SkipKubernetesOutputResourceValidation bool
	SkipObjectValidation                   bool
	SkipResourceDeletion                   bool
}

type RPTest struct {
	Options          RPTestOptions
	Name             string
	Description      string
	InitialResources []unstructured.Unstructured
	Steps            []TestStep
	PostDeleteVerify func(ctx context.Context, t *testing.T, ct RPTest)

	// RequiredFeatures specifies the optional features that are required
	// for this test to run.
	RequiredFeatures []RequiredFeature
}

type TestOptions struct {
	test.TestOptions
	DiscoveryClient discovery.DiscoveryInterface
}

// # Function Explanation
//
// NewTestOptions creates a new TestOptions object with the given testing.T object.
func NewTestOptions(t *testing.T) TestOptions {
	return TestOptions{TestOptions: test.NewTestOptions(t)}
}

// # Function Explanation
//
// NewRPTest creates a new RPTest instance with the given name, steps and initial resources.
func NewRPTest(t *testing.T, name string, steps []TestStep, initialResources ...unstructured.Unstructured) RPTest {
	return RPTest{
		Options:          NewRPTestOptions(t),
		Name:             name,
		Description:      name,
		Steps:            steps,
		InitialResources: initialResources,
	}
}

// # Function Explanation
//
// K8sSecretResource creates the secret resource from the given namespace, name, secretType and key-value pairs,
// for Initial Resource in NewRPTest().
func K8sSecretResource(namespace, name, secretType string, kv ...any) unstructured.Unstructured {
	if len(kv)%2 != 0 {
		panic("key value pairs must be even")
	}
	data := map[string]any{}
	for i := 0; i < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			panic("key must be string")
		}
		switch v := kv[i+1].(type) {
		case string:
			data[key] = []byte(v)
		case []byte:
			data[key] = v
		default:
			panic("value must be string or byte array")
		}
	}

	if secretType == "" {
		secretType = "opaque"
	}

	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"type":       secretType,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"data": data,
		},
	}
}

// # Function Explanation
//
// CreateInitialResources creates a namespace and creates initial resources from the InitialResources field of the
// RPTest struct. It returns an error if either of these operations fail.
func (ct RPTest) CreateInitialResources(ctx context.Context) error {
	if err := kubernetes.EnsureNamespace(ctx, ct.Options.K8sClient, ct.Name); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", ct.Name, err)
	}

	for _, r := range ct.InitialResources {
		if err := ct.Options.Client.Create(ctx, &r); err != nil {
			return fmt.Errorf("failed to create resource %#v:  %w", r, err)
		}
	}

	return nil
}

// # Function Explanation
//
// Method CleanUpExtensionResources deletes all resources in the given slice of unstructured objects.
func (ct RPTest) CleanUpExtensionResources(resources []unstructured.Unstructured) {
	for i := len(resources) - 1; i >= 0; i-- {
		_ = ct.Options.Client.Delete(context.TODO(), &resources[i])
	}
}

// # Function Explanation
//
// CheckRequiredFeatures checks the test environment for the features that the test requires and skips the test if not, otherwise
// returns an error if there is an issue.
func (ct RPTest) CheckRequiredFeatures(ctx context.Context, t *testing.T) {
	for _, feature := range ct.RequiredFeatures {
		var crd, message string
		switch feature {
		case FeatureDapr:
			crd = daprComponentCRD
			message = daprFeatureMessage
		case FeatureCSIDriver:
			crd = secretProviderClassesCRD
			message = csiDriverMessage
		default:
			panic(fmt.Sprintf("unsupported feature: %s", feature))
		}

		err := ct.Options.Client.Get(ctx, client.ObjectKey{Name: crd}, &apiextv1.CustomResourceDefinition{})
		if apierrors.IsNotFound(err) {
			t.Skip(message)
		} else if err != nil {
			require.NoError(t, err, "failed to check for required features")
		}
	}
}

func (ct RPTest) Test(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	ct.CheckRequiredFeatures(ctx, t)

	cli := radcli.NewCLI(t, ct.Options.ConfigFilePath)

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated, so they can run in parallel.
	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./logs/RPTest"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		_, err := validation.SaveContainerLogs(ctx, ct.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	// Start pod watchers for this test.
	watchers := map[string]watch.Interface{}
	for _, step := range ct.Steps {
		if step.K8sObjects == nil {
			continue
		}
		for ns := range step.K8sObjects.Namespaces {
			if _, ok := watchers[ns]; ok {
				continue
			}

			var err error
			watchers[ns], err = validation.SaveContainerLogs(ctx, ct.Options.K8sClient, ns, logPrefix)
			if err != nil {
				t.Errorf("failed to capture logs from radius controller: %v", err)
			}
		}
	}

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.
	require.GreaterOrEqual(t, len(ct.Steps), 1, "at least one step is required")
	defer ct.CleanUpExtensionResources(ct.InitialResources)
	err := ct.CreateInitialResources(ctx)
	require.NoError(t, err, "failed to create initial resources")

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

			if step.SkipKubernetesOutputResourceValidation {
				t.Logf("skipping validation of resources...")
			} else if step.RPResources == nil || len(step.RPResources.Resources) == 0 {
				require.Fail(t, "no resource set was specified and SkipResourceValidation == false, either specify a resource set or set SkipResourceValidation = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())
				validation.ValidateRPResources(ctx, t, step.RPResources, ct.Options.ManagementClient)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

			// Validate AWS resources if specified
			if step.AWSResources == nil || len(step.AWSResources.Resources) == 0 {
				t.Logf("no AWS resource set was specified, skipping validation")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())
				// Use the AWS CloudControl.Get method to validate that the resources are created
				validation.ValidateAWSResources(ctx, t, step.AWSResources, ct.Options.AWSClient)
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
		// Delete AWS resources if they were created. This delete logic is here because deleting a Radius application
		// will not delete the AWS resources that were created as part of the Bicep deployment.
		if step.AWSResources != nil && len(step.AWSResources.Resources) > 0 {
			for _, resource := range step.AWSResources.Resources {
				if !resource.SkipDeletion {
					t.Logf("deleting %s", resource.Name)

					// Use the AWS CloudControl.Delete method to delete the resource
					err := validation.DeleteAWSResource(ctx, &resource, ct.Options.AWSClient)
					if err != nil {
						t.Logf("failed to delete %s: %s", resource.Name, err)
					}

					// Ensure that the resource is deleted with retries
					notFound := false
					for attempt := 1; attempt <= AWSDeletionRetryLimit; attempt++ {
						t.Logf("validating deletion of AWS resource for %s (attempt %d/%d)", ct.Description, attempt, AWSDeletionRetryLimit)

						// Use AWS CloudControl.Get method to validate that the resource is deleted
						notFound, err = validation.IsAWSResourceNotFound(ctx, &resource, ct.Options.AWSClient)

						if notFound {
							t.Logf("AWS resource %s to be deleted was not found", resource.Identifier)
							break
						} else if err != nil {
							t.Logf("checking existence of resource %s failed with err: %s", resource.Name, err)
							break
						} else {
							// Wait for 10 seconds
							time.Sleep(10 * time.Second)
						}
					}

					require.Truef(t, notFound, "AWS resource %s was present, should be not found", resource.Identifier)
					t.Logf("finished validation of deletion of AWS resource %s for %s", resource.Name, ct.Description)
				} else {
					t.Logf("skipping deletion of %s", resource.Name)
				}
			}
		}

		if (step.RPResources == nil && step.SkipKubernetesOutputResourceValidation) || step.SkipResourceDeletion {
			continue
		}

		for _, resource := range step.RPResources.Resources {
			t.Logf("deleting %s", resource.Name)
			err := validation.DeleteRPResource(ctx, t, cli, ct.Options.ManagementClient, resource)
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

	// Stop all watchers for the tests.
	for _, watcher := range watchers {
		watcher.Stop()
	}

	t.Logf("finished cleanup phase of %s", ct.Description)
}
