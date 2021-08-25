// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/validation"
)

func Test_ContainerHttpBinding(t *testing.T) {
	template := "testdata/kubernetes-resources-container-httpbinding.bicep"
	application := "kubernetes-resources-container-httpbinding"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForComponent(application, "frontend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_ContainerManualScale(t *testing.T) {
	template := "testdata/kubernetes-resources-container-manualscale.bicep"
	application := "kubernetes-resources-container-manualscale"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForComponent(application, "frontend"),
						// Verify two backend pods are created.
						validation.NewK8sObjectForComponent(application, "backend"),
						validation.NewK8sObjectForComponent(application, "backend"),
					},
				},
			},
		},
	})

	test.Test(t)
}
