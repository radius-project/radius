// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_ContainerReadinessLiveness(t *testing.T) {
	template := "testdata/corerp-resources-container-liveness-readiness.bicep"
	name := "corerp-resources-container-live-ready"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-live-ready",
						Type: validation.ContainersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "ctnr-live-ready"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

/*
func Test_ContainerReadinessLiveness(t *testing.T) {
	application := "azure-resources-container-readiness-liveness"
	template := "testdata/azure-resources-container-readiness-liveness.bicep"
	test := azure.NewApplicationTest(t, application, []azure.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			AzureResources: &validation.AzureResourceSet{
				Resources: []validation.ExpectedResource{
					// Intentionally Empty
				},
			},
			Objects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "backend"),
					},
				},
			},
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "backend",
						ResourceType:    containerv1alpha3.ResourceType,
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, at azure.ApplicationTest) {
				// Verify there are two pods created for backend.
				labelset := kubernetes.MakeSelectorLabels(application, "backend")

				matches, err := at.Options.K8sClient.CoreV1().Pods(application).List(context.Background(), metav1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list pods")
				require.Lenf(t, matches.Items, 1, "items should contain two match, instead it had: %+v", matches.Items)

				// Verify readiness probe
				require.Equal(t, "/healthz", matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Path)
				require.Equal(t, intstr.FromInt(3000), matches.Items[0].Spec.Containers[0].ReadinessProbe.HTTPGet.Port)
				require.Equal(t, int32(3), matches.Items[0].Spec.Containers[0].ReadinessProbe.InitialDelaySeconds)
				require.Equal(t, int32(4), matches.Items[0].Spec.Containers[0].ReadinessProbe.FailureThreshold)
				require.Equal(t, int32(20), matches.Items[0].Spec.Containers[0].ReadinessProbe.PeriodSeconds)
				require.Nil(t, matches.Items[0].Spec.Containers[0].ReadinessProbe.TCPSocket)
				require.Nil(t, matches.Items[0].Spec.Containers[0].ReadinessProbe.Exec)

				// Verify liveness probe
				require.Equal(t, []string{"ls", "/tmp"}, matches.Items[0].Spec.Containers[0].LivenessProbe.Exec.Command)
				require.Equal(t, int32(0), matches.Items[0].Spec.Containers[0].LivenessProbe.InitialDelaySeconds)
				require.Equal(t, int32(3), matches.Items[0].Spec.Containers[0].LivenessProbe.FailureThreshold)
				require.Equal(t, int32(10), matches.Items[0].Spec.Containers[0].LivenessProbe.PeriodSeconds)
				require.Nil(t, matches.Items[0].Spec.Containers[0].LivenessProbe.TCPSocket)
				require.Nil(t, matches.Items[0].Spec.Containers[0].LivenessProbe.HTTPGet)
			},
		},
	})

	test.Test(t)
}
*/
