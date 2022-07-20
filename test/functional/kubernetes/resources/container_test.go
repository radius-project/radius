// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/providers"
	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_ContainerHttpBinding(t *testing.T) {
	template := "testdata/kubernetes-resources-container-httpbinding.bicep"
	application := "kubernetes-resources-container-httpbinding"
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, rest.ResourceType{Type: resourcekinds.Secret, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "frontend"),
						validation.NewK8sPodForResource(application, "backend"),
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
	test := kubernetes.NewApplicationTest(t, application, []kubernetes.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "frontend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDSecret:     validation.NewOutputResource(outputresource.LocalIDSecret, rest.ResourceType{Type: resourcekinds.Secret, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "backend",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, rest.ResourceType{Type: resourcekinds.Deployment, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, rest.ResourceType{Type: resourcekinds.Service, Provider: providers.ProviderKubernetes}, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "frontend"),
						// Verify two backend pods are created.
						validation.NewK8sPodForResource(application, "backend"),
						validation.NewK8sPodForResource(application, "backend"),
					},
				},
			},
		},
	})

	test.Test(t)
}
