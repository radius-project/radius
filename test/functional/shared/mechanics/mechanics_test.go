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
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_NestedModules(t *testing.T) {
	template := "testdata/corerp-mechanics-nestedmodules.bicep"
	name := "corerp-mechanics-nestedmodules"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-mechanics-nestedmodules-outerapp-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-mechanics-nestedmodules-innerapp-app",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})

	test.Test(t)
}

func Test_RedeployWithAnotherResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-with-another-resource"
	appNamespace := "default-corerp-mechanics-redeploy-with-another-resource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withanotherresource.step%d.bicep"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicsa",
						Type: validation.ContainersResource,
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
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicsb",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mechanicsc",
						Type: validation.ContainersResource,
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

	test.Test(t)
}

func Test_RedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withupdatedresource"
	appNamespace := "default-corerp-mechanics-redeploy-withupdatedresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withupdatedresource.step%d.bicep"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicsd",
						Type: validation.ContainersResource,
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
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicsd",
						Type: validation.ContainersResource,
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test shared.RPTest) {
				labelset := kubernetes.MakeSelectorLabels(name, "mechanicsd")

				deployments, err := test.Options.K8sClient.AppsV1().Deployments(appNamespace).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})

				require.NoError(t, err, "failed to list deployments")
				require.Len(t, deployments.Items, 1, "expected 1 deployment")
				deployment := deployments.Items[0]
				envVar := deployment.Spec.Template.Spec.Containers[0].Env[0]
				require.Equal(t, "TEST", envVar.Name, "expected env var to be updated")
				require.Equal(t, "updated", envVar.Value, "expected env var to be updated")
			},
		},
	})
	test.Test(t)
}

func Test_RedeployWithTwoSeparateResourcesKeepsResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withtwoseparateresource"
	appNamespace := "default-corerp-mechanics-redeploy-withtwoseparateresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withtwoseparateresource.step%d.bicep"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicse",
						Type: validation.ContainersResource,
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
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mechanicse",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mechanicsf",
						Type: validation.ContainersResource,
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

	test.Test(t)
}

func Test_CommunicationCycle(t *testing.T) {
	name := "corerp-mechanics-communication-cycle"
	appNamespace := "default-corerp-mechanics-communication-cycle"
	template := "testdata/corerp-mechanics-communication-cycle.bicep"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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

	// We've avoiding including resource IDs here because they can change depending on how the run is
	// configured.
	validate := step.ValidateAllDetails("DeploymentFailed", []step.DeploymentErrorDetail{
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "BadRequest",
					MessageContains: "has invalid Applications.Core/applications resource type.",
				},
			},
		},
		{
			Code: "ResourceDeploymentFailure",
			Details: []step.DeploymentErrorDetail{
				{
					Code:            "BadRequest",
					MessageContains: "application ID \"not_an_id\" for the resource",
				},
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, functional.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})

	test.Test(t)
}
