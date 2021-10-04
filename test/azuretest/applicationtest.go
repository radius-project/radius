// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package azuretest

import (
	"context"
	"testing"

	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/testcontext"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

type Step struct {
	Executor           StepExecutor
	AzureResources     *validation.AzureResourceSet
	Components         *validation.ComponentSet
	Pods               *validation.K8sObjectSet
	PostStepVerify     func(ctx context.Context, t *testing.T, at ApplicationTest)
	SkipAzureResources bool
	SkipComponents     bool
	SkipPods           bool
}

type StepExecutor interface {
	GetDescription() string
	Execute(ctx context.Context, t *testing.T, options TestOptions)
}

type ApplicationTest struct {
	Options          TestOptions
	Application      string
	Description      string
	SkipDeletion     bool
	Steps            []Step
	PostDeleteVerify func(ctx context.Context, t *testing.T, at ApplicationTest)
}

func NewApplicationTest(t *testing.T, application string, steps []Step) ApplicationTest {
	return ApplicationTest{
		Options:     NewTestOptions(t),
		Application: application,
		Description: application,
		Steps:       steps,
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

func (at ApplicationTest) Test(t *testing.T) {
	ctx, cancel := testcontext.GetContext(t)
	defer cancel()

	// This runs each application deployment step as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

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

			t.Logf("running step %d of %d: %s", i+1, len(at.Steps), step.Executor.GetDescription())
			step.Executor.Execute(ctx, t, at.Options)
			t.Logf("finished running step %d of %d: %s", i+1, len(at.Steps), step.Executor.GetDescription())

			if step.AzureResources == nil && step.SkipAzureResources {
				t.Logf("skipping validation of Azure resources..")
			} else if step.AzureResources == nil {
				require.Fail(t, "no azure resource set was specified and SkipAzureResources == false, either specify a resource set or set SkipAzureResources = true ")
			} else {
				// Validate that all expected Azure resources are created
				t.Logf("validating Azure resources for %s", step.Executor.GetDescription())
				validation.ValidateAzureResourcesCreated(ctx, t, at.Options.ARMAuthorizer, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, at.Application, *step.AzureResources)
				t.Logf("finished validating Azure resources for %s", step.Executor.GetDescription())
			}

			if step.Components == nil && step.SkipComponents {
				t.Logf("skipping validation of components...")
			} else if step.Components == nil {
				require.Fail(t, "no component set was specified and SkipComponents == false, either specify a component set or set SkipComponents = true ")
			} else {
				// Validate that all expected output resources are created
				t.Logf("validating output resources for %s", step.Executor.GetDescription())
				validation.ValidateOutputResources(t, at.Options.ARMAuthorizer, at.Options.ARMConnection, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, *step.Components)
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

	if at.SkipDeletion {
		t.Logf("skipping deletion of %s...", at.Description)
		return
	}

	t.Logf("beginning cleanup phase of %s", at.Description)

	// Cleanup code here will run regardless of pass/fail of subtests
	t.Logf("deleting %s", at.Description)
	err := cli.ApplicationDelete(ctx, at.Application)
	require.NoErrorf(t, err, "failed to delete %s", at.Description)
	t.Logf("finished deleting %s", at.Description)

	// We run the validation code based on the final step
	last := at.Steps[len(at.Steps)-1]

	// We don't need to validate the components because they are already gone.

	if last.SkipAzureResources {
		t.Logf("skipping validation of Azure resources..")
	} else {
		// Validate that all expected Azure resources were deleted
		t.Logf("validating deletion of Azure resources for %s", last.Executor.GetDescription())
		validation.ValidateAzureResourcesDeleted(ctx, t, at.Options.ARMAuthorizer, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, at.Application, *last.AzureResources)
		t.Logf("finished validating deletion of Azure resources for %s", last.Executor.GetDescription())
	}

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
