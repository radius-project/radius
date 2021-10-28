// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package mechanics_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Azure/radius/pkg/kubernetes"
	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/renderers/containerv1alpha3"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Tests that we can add a component to a deployed application
// by redeploying with more components.
func Test_RedeployWithAnotherComponent(t *testing.T) {
	application := "kubernetes-mechanics-redeploy-withanothercomponent"
	templateFmt := "testdata/kubernetes-mechanics-redeploy-withanothercomponent.step%d.bicep"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 1)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
					},
				},
			},
		},
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 2)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "b",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
						validation.NewK8sObjectForResource(application, "b"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RedeployWithUpdatedComponentUpdatesComponent(t *testing.T) {
	application := "kubernetes-mechanics-redeploy-withupdatedcomponent"
	templateFmt := "testdata/kubernetes-mechanics-redeploy-withupdatedcomponent.step%d.bicep"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 1)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
					},
				},
			},
		},
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 2)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test kubernetestest.ApplicationTest) {
				labelset := kubernetes.MakeSelectorLabels(application, "a")

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
	})
	test.Test(t)
}

func Test_RedeployWitTwoSeparateComponentsKeepsComponent(t *testing.T) {
	application := "kubernetes-mechanics-redeploy-withtwoseparatecomponent"
	templateFmt := "testdata/kubernetes-mechanics-redeploy-withtwoseparatecomponent.step%d.bicep"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 1)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
					},
				},
			},
		},
		{
			Executor: kubernetestest.NewDeployStepExecutor(fmt.Sprintf(templateFmt, 2)),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "a",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "b",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "a"),
						validation.NewK8sObjectForResource(application, "b"),
					},
				},
			},
		},
	})

	test.Test(t)
}
