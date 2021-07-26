// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package kubernetestest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	radiusv1alpha1 "github.com/Azure/radius/pkg/kubernetes/api/v1alpha1"
	"github.com/Azure/radius/pkg/rad"
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	k8s "k8s.io/client-go/kubernetes"
)

type Row struct {
	Application *radiusv1alpha1.Application
	Components  *[]TestComponent
	Description string
	Pods        validation.K8sObjectSet
}

func (r Row) GetComponents() (*[]radiusv1alpha1.Component, error) {
	var components []radiusv1alpha1.Component

	for _, testComponent := range *r.Components {
		component, err := testComponent.GetComponent()
		if err != nil {
			return nil, err
		}
		components = append(components, component)
	}

	return &components, nil
}

// A test only representation of a component, making it easier
// to write input for (don't need to muck with RawExtension for json)
type TestComponent struct {
	TypeMeta   metav1.TypeMeta
	ObjectMeta metav1.ObjectMeta
	Spec       TestComponentSpec
}

type TestComponentSpec struct {
	Kind      string
	Hierarchy []string
	Run       map[string]interface{}
	Bindings  map[string]interface{}
	Config    map[string]interface{}
	Uses      []map[string]interface{}
	Traits    []map[string]interface{}
}

func (tc TestComponent) GetComponent() (radiusv1alpha1.Component, error) {
	// handle defaults
	if tc.Spec.Run == nil {
		tc.Spec.Run = map[string]interface{}{}
	}
	if tc.Spec.Bindings == nil {
		tc.Spec.Bindings = map[string]interface{}{}
	}
	if tc.Spec.Config == nil {
		tc.Spec.Config = map[string]interface{}{}
	}
	if tc.Spec.Uses == nil {
		tc.Spec.Uses = []map[string]interface{}{}
	}

	bindingJson, err := json.Marshal(tc.Spec.Bindings)
	if err != nil {
		return radiusv1alpha1.Component{}, err
	}
	runJson, err := json.Marshal(tc.Spec.Run)
	if err != nil {
		return radiusv1alpha1.Component{}, err
	}

	uses := []runtime.RawExtension{}

	for _, use := range tc.Spec.Uses {
		useJson, err := json.Marshal(use)
		if err != nil {
			return radiusv1alpha1.Component{}, err
		}
		uses = append(uses, runtime.RawExtension{Raw: useJson})
	}

	traits := []runtime.RawExtension{}
	for _, trait := range tc.Spec.Traits {
		traitJson, err := json.Marshal(trait)
		if err != nil {
			return radiusv1alpha1.Component{}, err
		}
		traits = append(traits, runtime.RawExtension{Raw: traitJson})
	}

	configJson, err := json.Marshal(tc.Spec.Config)
	if err != nil {
		return radiusv1alpha1.Component{}, err
	}
	return v1alpha1.Component{
		TypeMeta:   tc.TypeMeta,
		ObjectMeta: tc.ObjectMeta,
		Spec: v1alpha1.ComponentSpec{
			Kind:      tc.Spec.Kind,
			Run:       &runtime.RawExtension{Raw: runJson},
			Bindings:  runtime.RawExtension{Raw: bindingJson},
			Hierarchy: tc.Spec.Hierarchy,
			Uses:      &uses,
			Traits:    &traits,
			Config:    &runtime.RawExtension{Raw: configJson},
		},
	}, nil

}

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
	config, err := rad.LoadConfig("")
	require.NoError(t, err, "failed to read radius config")

	k8s, err := utils.GetKubernetesClient()
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
	ctx, cancel := utils.GetContext(t)
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
				// Will be done by https://github.com/Azure/radius/issues/760
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
	err := cli.ApplicationDelete(ctx, at.Application)
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
