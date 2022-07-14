// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"context"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/project-radius/radius/pkg/kubernetes"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
)

func Test_NestedModules(t *testing.T) {
	template := "testdata/corerp-mechanics-nestedmodules.bicep"
	name := "corerp-mechanics-nestedmodules"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
	t.Skip("Will re-enable after all components are completed for Private Preview. Ref: https://github.com/project-radius/radius/issues/2736")

	name := "corerp-mechanics-redeploy-withanotherresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withanotherresource.step%d.bicep"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-b",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-b"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	t.Skip("Will re-enable after all components are completed for Private Preview. Ref: https://github.com/project-radius/radius/issues/2736")

	name := "corerp-mechanics-redeploy-withupdatedresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withupdatedresource.step%d.bicep"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				labelset := kubernetes.MakeSelectorLabels(name, "corerp-mechanics-redeploy-withanotherresource-a")

				deployments, err := test.Options.K8sClient.AppsV1().Deployments(name).List(context.Background(), metav1.ListOptions{
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

func Test_RedeployWitTwoSeparateResourcesKeepsResource(t *testing.T) {
	t.Skip("Will re-enable after all components are completed for Private Preview. Ref: https://github.com/project-radius/radius/issues/2736")

	name := "corerp-mechanics-redeploy-withtwoseparateresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withtwoseparateresource.step%d.bicep"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-a",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-mechanics-redeploy-withanotherresource-b",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-a"),
						validation.NewK8sPodForResource(name, "corerp-mechanics-redeploy-withanotherresource-b"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_CommunicationCycle(t *testing.T) {
	t.Skip("Will re-enable after all components are completed for Private Preview. Ref: https://github.com/project-radius/radius/issues/2736")

	name := "corerp-mechanics-communication-cycle"
	template := "testdata/corerp-mechanics-communication-cycle.bicep"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-communication-cycle-a",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-mechanics-communication-cycle-a-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-mechanics-communication-cycle-b",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-mechanics-communication-cycle-b-route",
						Type: validation.HttpRoutesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-mechanics-communication-cycle-a"),
						validation.NewK8sPodForResource(name, "corerp-mechanics-communication-cycle-b"),
					},
				},
			},
		},
	})

	test.Test(t)
}
