// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetestest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Azure/radius/pkg/cli"
	"github.com/Azure/radius/pkg/cli/kubernetes"
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/testcontext"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/restmapper"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
)

type Step struct {
	Executor               StepExecutor
	RadiusResources        *validation.ResourceSet
	K8sOutputResources     []unstructured.Unstructured
	Pods                   *validation.K8sObjectSet
	Ingress                *validation.K8sObjectSet
	Services               *validation.K8sObjectSet
	PostStepVerify         func(ctx context.Context, t *testing.T, at ApplicationTest)
	SkipOutputResources    bool
	SkipResourceValidation bool
}

type StepExecutor interface {
	GetDescription() string
	Execute(ctx context.Context, t *testing.T, options TestOptions)
}

type ApplicationTest struct {
	Options          TestOptions
	Application      string
	Description      string
	InitialResources []unstructured.Unstructured
	Steps            []Step
	PostDeleteVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
}

type TestOptions struct {
	ConfigFilePath  string
	K8sClient       *k8s.Clientset
	DynamicClient   dynamic.Interface
	DiscoveryClient discovery.DiscoveryInterface
}

func NewTestOptions(t *testing.T) TestOptions {
	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	k8sconfig, err := kubernetes.ReadKubeConfig()
	require.NoError(t, err, "failed to read k8s config")

	k8s, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes client")

	dynamicClient, err := kubernetes.CreateDynamicClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes dyamic client")

	return TestOptions{
		ConfigFilePath: config.ConfigFileUsed(),
		K8sClient:      k8s,
		DynamicClient:  dynamicClient,
	}
}

func (at ApplicationTest) CollectAllNamespaces() []string {
	all := map[string]bool{}
	for _, step := range at.Steps {
		if step.Pods != nil {
			for ns := range step.Pods.Namespaces {
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

var _ StepExecutor = (*DeployStepExecutor)(nil)

type DeployStepExecutor struct {
	Description string
	Template    string
	Parameters  []string
}

func NewDeployStepExecutor(template string, parameters ...string) *DeployStepExecutor {
	return &DeployStepExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
		Parameters:  parameters,
	}
}

func (d *DeployStepExecutor) GetDescription() string {
	return d.Description
}

func (d *DeployStepExecutor) Execute(ctx context.Context, t *testing.T, options TestOptions) {
	cwd, err := os.Getwd()
	require.NoError(t, err)

	templateFilePath := filepath.Join(cwd, d.Template)
	t.Logf("deploying %s from file %s", d.Description, d.Template)
	cli := radcli.NewCLI(t, options.ConfigFilePath)
	err = cli.Deploy(ctx, templateFilePath, d.Parameters...)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

func NewApplicationTest(t *testing.T, application string, steps []Step, initialResources ...unstructured.Unstructured) ApplicationTest {
	return ApplicationTest{
		Options:          NewTestOptions(t),
		Application:      application,
		Description:      application,
		InitialResources: initialResources,
		Steps:            steps,
	}
}

func (at ApplicationTest) CreateInitialResources() error {
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(at.Options.K8sClient.Discovery()))
	for _, r := range at.InitialResources {
		mapping, err := restMapper.RESTMapping(r.GroupVersionKind().GroupKind(), r.GroupVersionKind().Version)
		if err != nil {
			return fmt.Errorf("unknown kind %q: %w", r.GroupVersionKind().String(), err)
		}
		if mapping.Scope == meta.RESTScopeNamespace {
			_, err = at.Options.DynamicClient.Resource(mapping.Resource).
				Namespace(r.GetNamespace()).
				Create(context.TODO(), &r, v1.CreateOptions{})
		} else {
			_, err = at.Options.DynamicClient.Resource(mapping.Resource).
				Create(context.TODO(), &r, v1.CreateOptions{})
		}
		if err != nil {
			return fmt.Errorf("failed to create %q resource %#v:  %w", mapping.Resource.String(), r, err)
		}
	}
	return nil
}

func (at ApplicationTest) CleanUpResources(resources []unstructured.Unstructured) {
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
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	// Capture all logs from all pods (only run one of these as it will monitor everything)
	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

	logPrefix := os.Getenv(ContainerLogPathEnvVar)
	if logPrefix == "" {
		logPrefix = "./container_logs"
	}

	// Only start capturing controller logs once.
	radiusControllerLogSync.Do(func() {
		err := validation.SaveLogsForController(ctx, at.Options.K8sClient, "radius-system", logPrefix)
		if err != nil {
			t.Errorf("failed to capture logs from radius controller: %v", err)
		}
	})

	err := validation.SaveLogsForApplication(ctx, at.Options.K8sClient, "default", logPrefix+"/"+at.Application, at.Application)
	if err != nil {
		t.Errorf("failed to capture logs from radius pods %v", err)
	}

	cli := radcli.NewCLI(t, at.Options.ConfigFilePath)

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	require.GreaterOrEqual(t, len(at.Steps), 1, "at least one step is required")
	defer at.CleanUpResources(at.InitialResources)
	err = at.CreateInitialResources()
	require.NoError(t, err, "failed to create initial resources")
	success := true
	for i, step := range at.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			defer at.CleanUpResources(step.K8sOutputResources)
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options)
			t.Logf("finished running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())

			if step.RadiusResources == nil && step.SkipOutputResources {
				t.Logf("skipping validation of components...")
			} else if step.RadiusResources == nil {
				require.Fail(t, "no component set was specified and SkipComponents == false, either specify a component set or set SkipComponents = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())

				// TODO: create k8s client for validating output resources
				// https://github.com/Azure/radius/issues/778
				// validation.ValidateOutputResources(t, at.Options.ARMConnection, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, *step.Components)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

			if step.SkipResourceValidation {
				t.Logf("skipping validation of resources...")
			} else if step.Pods == nil && step.Ingress == nil && step.Services == nil && len(step.K8sOutputResources) == 0 {
				require.Fail(t, "no resources specified and SkipResourceValidation == false, either specify a resource set or set SkipResourceValidation = true ")
			} else {
				if step.Pods != nil {
					t.Logf("validating creation of pods for %s", step.Executor.GetDescription())
					validation.ValidatePodsRunning(ctx, t, at.Options.K8sClient, *step.Pods)
					t.Logf("finished creation of validating pods for %s", step.Executor.GetDescription())
				}

				if step.Ingress != nil {
					t.Logf("validating creation of ingress for %s", step.Executor.GetDescription())
					validation.ValidateIngressesRunning(ctx, t, at.Options.K8sClient, *step.Ingress)
					t.Logf("finished creation of validating ingress for %s", step.Executor.GetDescription())
				}

				if step.Services != nil {
					t.Logf("validating creation of services for %s", step.Executor.GetDescription())
					validation.ValidateServicesRunning(ctx, t, at.Options.K8sClient, *step.Services)
					t.Logf("finished creation of validating services for %s", step.Executor.GetDescription())
				}

				if step.K8sOutputResources != nil {
					t.Logf("validating creation of resources for %s", step.Executor.GetDescription())
					validation.ValidateResourcesCreated(ctx, t, at.Options.K8sClient, at.Options.DynamicClient, step.K8sOutputResources)
					t.Logf("kubernetes extension resources were created correctly for %s", step.Executor.GetDescription())
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
			validation.ValidateNoPodsInNamespace(ctx, t, at.Options.K8sClient, ns)
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
