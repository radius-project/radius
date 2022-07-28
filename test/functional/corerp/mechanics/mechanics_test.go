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

	requiredSecrets := map[string]map[string]string{}

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
	}, requiredSecrets)

	test.Test(t)
}

func Test_RedeployWithAnotherResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-with-another-resource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withanotherresource.step%d.bicep"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-with-another-resource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicsa",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-with-another-resource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicsa"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-with-another-resource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicsb",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-with-another-resource",
					},
					{
						Name:    "mechanicsc",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-with-another-resource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicsb"),
						validation.NewK8sPodForResource(name, "mechanicsc"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_RedeployWithUpdatedResourceUpdatesResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withupdatedresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withupdatedresource.step%d.bicep"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withupdatedresource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicsd",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-withupdatedresource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicsd"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withupdatedresource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicsd",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-withupdatedresource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicsd"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test corerp.CoreRPTest) {
				labelset := kubernetes.MakeSelectorLabels(name, "mechanicsd")

				deployments, err := test.Options.K8sClient.AppsV1().Deployments("default").List(context.Background(), metav1.ListOptions{
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
	}, requiredSecrets)
	test.Test(t)
}

func Test_RedeployWithTwoSeparateResourcesKeepsResource(t *testing.T) {
	name := "corerp-mechanics-redeploy-withtwoseparateresource"
	templateFmt := "testdata/corerp-mechanics-redeploy-withtwoseparateresource.step%d.bicep"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 1), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withtwoseparateresource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicse",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-withtwoseparateresource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicse"),
					},
				},
			},
		},
		{
			Executor: step.NewDeployExecutor(fmt.Sprintf(templateFmt, 2), functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-redeploy-withtwoseparateresource",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "mechanicse",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-withtwoseparateresource",
					},
					{
						Name:    "mechanicsf",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-redeploy-withtwoseparateresource",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicse"),
						validation.NewK8sPodForResource(name, "mechanicsf"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_CommunicationCycle(t *testing.T) {
	name := "corerp-mechanics-communication-cycle"
	template := "testdata/corerp-mechanics-communication-cycle.bicep"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-communication-cycle",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "routea",
						Type:    validation.HttpRoutesResource,
						AppName: "corerp-mechanics-communication-cycle",
					},
					{
						Name:    "mechanicsg",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-communication-cycle",
					},
					{
						Name:    "routeb",
						Type:    validation.HttpRoutesResource,
						AppName: "corerp-mechanics-communication-cycle",
					},
					{
						Name:    "cyclea",
						Type:    validation.ContainersResource,
						AppName: "corerp-mechanics-communication-cycle",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mechanicsg"),
						validation.NewK8sPodForResource(name, "cyclea"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_InvalidResourceIDs(t *testing.T) {
	name := "corerp-mechanics-invalid-resourceids"
	template := "testdata/corerp-mechanics-invalid-resourceids.bicep"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-mechanics-invalid-resourceids",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	}, requiredSecrets)

	test.Test(t)
}
