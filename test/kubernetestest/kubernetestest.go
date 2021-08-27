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
	k8s "k8s.io/client-go/kubernetes"
)

var radiusControllerLogSync sync.Once

const (
	ContainerLogPathEnvVar = "RADIUS_CONTAINER_LOG_PATH"
)

type Step struct {
	Executor       StepExecutor
	Components     *validation.ComponentSet
	Pods           *validation.K8sObjectSet
	PostStepVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
	SkipComponents bool
	SkipPods       bool
}

type StepExecutor interface {
	GetDescription() string
	Execute(ctx context.Context, t *testing.T, options TestOptions)
}

type ApplicationTest struct {
	Options          TestOptions
	Application      string
	Description      string
	Steps            []Step
	PostDeleteVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
}

type TestOptions struct {
	ConfigFilePath string
	K8sClient      *k8s.Clientset
}

func NewTestOptions(t *testing.T) TestOptions {
	config, err := cli.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	k8sconfig, err := kubernetes.ReadKubeConfig()
	require.NoError(t, err, "failed to read k8s config")

	k8s, _, err := kubernetes.CreateTypedClient(k8sconfig.CurrentContext)
	require.NoError(t, err, "failed to create kubernetes client")

	return TestOptions{
		ConfigFilePath: config.ConfigFileUsed(),
		K8sClient:      k8s,
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
}

func NewDeployStepExecutor(template string) *DeployStepExecutor {
	return &DeployStepExecutor{
		Description: fmt.Sprintf("deploy %s", template),
		Template:    template,
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
	err = cli.Deploy(ctx, templateFilePath)
	require.NoErrorf(t, err, "failed to deploy %s", d.Description)
	t.Logf("finished deploying %s from file %s", d.Description, d.Template)
}

func NewApplicationTest(t *testing.T, application string, steps []Step) ApplicationTest {
	return ApplicationTest{
		Options:     NewTestOptions(t),
		Application: application,
		Description: application,
		Steps:       steps,
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
			t.Errorf("failed to capture logs from radius controller: %w", err)
		}
	})

	err := validation.SaveLogsForApplication(ctx, at.Options.K8sClient, "default", logPrefix+"/"+at.Application, at.Application)
	if err != nil {
		t.Errorf("failed to capture logs from radius pods %w", err)
	}

	cli := radcli.NewCLI(t, at.Options.ConfigFilePath)

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	require.GreaterOrEqual(t, len(at.Steps), 1, "at least one step is required")

	success := true
	for i, step := range at.Steps {
		success = t.Run(step.Executor.GetDescription(), func(t *testing.T) {
			if !success {
				t.Skip("skipping due to previous step failure")
				return
			}

			t.Logf("running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options)
			t.Logf("finished running step %d of %d: %s", i, len(at.Steps), step.Executor.GetDescription())

			if step.Components == nil && step.SkipComponents {
				t.Logf("skipping validation of components...")
			} else if step.Components == nil {
				require.Fail(t, "no component set was specified and SkipComponents == false, either specify a component set or set SkipComponents = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())

				// TODO: create k8s client for validating output resources
				// https://github.com/Azure/radius/issues/778
				// validation.ValidateOutputResources(t, at.Options.ARMConnection, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, *step.Components)
				t.Logf("finished validating output resources for %s", step.Executor.GetDescription())
			}

			if step.Pods == nil && step.SkipPods {
				t.Logf("skipping validation of pods...")
			} else if step.Pods == nil {
				require.Fail(t, "no pod set was specified and SkipPods == false, either specify a pod set or set SkipPods = true ")
			} else {
				// ValidatePodsRunning triggers its own assertions, no need to handle errors
				t.Logf("validating creation of pods for %s", step.Executor.GetDescription())
				validation.ValidatePodsRunning(ctx, t, at.Options.K8sClient, *step.Pods)
				t.Logf("finished creation of validating pods for %s", step.Executor.GetDescription())
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

	// Cleanup code here will run regardless of pass/fail of subtests
	t.Logf("deleting %s", at.Description)
	err = cli.ApplicationDelete(ctx, at.Application)
	require.NoErrorf(t, err, "failed to delete %s", at.Description)
	t.Logf("finished deleting %s", at.Description)

	// We run the validation code based on the final step
	last := at.Steps[len(at.Steps)-1]

	if last.SkipPods {
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
