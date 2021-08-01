// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package container_test

import (
	"testing"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/validation"
)

func Test_Mongo(t *testing.T) {
	template := "testdata/kubernetes-resources-mongo.bicep"
	application := "kubernetes-resources-mongo"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			Components: &validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: application,
						ComponentName:   "todoapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, outputresource.KindKubernetes, true),
							outputresource.LocalIDService:    validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true),
						},
					},
					{
						ApplicationName: application,
						ComponentName:   "db",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDSecret:      validation.NewOutputResource(outputresource.LocalIDSecret, outputresource.TypeKubernetes, outputresource.KindKubernetes, true),
							outputresource.LocalIDStatefulSet: validation.NewOutputResource(outputresource.LocalIDStatefulSet, outputresource.TypeKubernetes, outputresource.KindKubernetes, true),
							outputresource.LocalIDService:     validation.NewOutputResource(outputresource.LocalIDService, outputresource.TypeKubernetes, outputresource.KindKubernetes, true),
						},
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sObjectForComponent(application, "todoapp"),
						validation.NewK8sObjectForComponent(application, "db"),
					},
				},
			},
		},
	})

	test.Test(t)
}
