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

func Test_DaprStateStore(t *testing.T) {
	template := "testdata/kubernetes-resources-statestore-managed.bicep"
	application := "kubernetes-resources-statestore-managed"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "sender",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true, false, validation.ExpectedOutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "statestore",
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
						validation.NewK8sObjectForComponent(application, "sender"),
						validation.NewK8sObjectForComponent(application, "statestore"),
					},
				},
			},
		},
	})

	test.Test(t)
}
