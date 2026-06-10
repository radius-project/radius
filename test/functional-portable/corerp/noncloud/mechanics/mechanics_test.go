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

package mechanics_test

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_NestedModules(t *testing.T) {
	template := "testdata/corerp-mechanics-nestedmodules.bicep"
	name := "corerp-mechanics-nestedmodules"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-mechanics-nestedmodules-outerapp-app",
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "corerp-mechanics-nestedmodules-innerapp-app",
						Type: validation.CoreApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_RedeployWithAnotherResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-with-another-resource"
	appNamespace := name
	templateFmt := "testdata/corerp-mechanics-redeploy-withanotherresource.step%d.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicsa",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicsa"),
					},
				},
			},
		},
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicsb",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "mechanicsc",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicsb"),
						validation.NewK8sPodForResource(name, "mechanicsc"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))
	test.Steps[1].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_RedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withupdatedresource"
	appNamespace := name
	templateFmt := "testdata/corerp-mechanics-redeploy-withupdatedresource.step%d.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicsd",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicsd"),
					},
				},
			},
		},
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicsd",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicsd"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				labelset := kubernetes.MakeSelectorLabels(name, "mechanicsd")

				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})

				require.NoError(t, err, "failed to list deployments")
				require.Len(t, deployments.Items, 1, "expected 1 deployment")
				deployment := deployments.Items[0]

				var found bool
				for _, envVar := range deployment.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == "TEST" {
						found = true
						require.Equal(t, "updated", envVar.Value, "expected env var TEST to be updated")
						break
					}
				}
				require.True(t, found, "expected env var TEST to be present on the redeployed container")
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))
	test.Steps[1].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_RedeployWithTwoSeparateResourcesKeepsResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withtwoseparateresource"
	appNamespace := name
	templateFmt := "testdata/corerp-mechanics-redeploy-withtwoseparateresource.step%d.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicse",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicse"),
					},
				},
			},
		},
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: "mechanicse",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: "mechanicsf",
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicse"),
						validation.NewK8sPodForResource(name, "mechanicsf"),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))
	test.Steps[1].Executor = step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

func Test_CommunicationCycle(t *testing.T) {
	name := "corerp-mechanics-communication-cycle"
	appNamespace := "default-corerp-mechanics-communication-cycle"
	template := "testdata/corerp-mechanics-communication-cycle.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicsg",
						Type: validation.ContainersResource,
						App:  "corerp-mechanics-communication-cycle",
					},
					{
						Name: "cyclea",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "mechanicsg"),
						validation.NewK8sPodForResource(name, "cyclea"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_InvalidResourceIDs(t *testing.T) {
	name := "corerp-mechanics-invalid-resourceids"
	template := "testdata/corerp-mechanics-invalid-resourceids.bicep"

	// The container references an invalid application resource ID ("not_an_id"). The deployment is
	// expected to fail because the application ID cannot be parsed as a valid resource id. As with
	// the original Applications.Core version of this test, the failed container resource is left
	// behind (it cannot be cascade-deleted because it has no valid owning application), so resource
	// and object validation are skipped.
	validate := step.ValidateAllDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "RecipeDeploymentFailed",
					MessageContains: "is not a valid resource id",
				},
			},
		},
	})

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployErrorExecutor(template, validate, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
