// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
)

func Test_Redis(t *testing.T) {
	template := "testdata/kubernetes-resources-redis-managed.bicep"
	application := "kubernetes-resources-redis-managed"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "container-cool-cache",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "todoapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForResource(application, "container-cool-cache"),
						validation.NewK8sObjectForResource(application, "todoapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
