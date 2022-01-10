// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
)

func TestK8sModule(t *testing.T) {
	template := "testdata/kubernetes-module/main.bicep"
	application := "kubernetes-module"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "application",
					},
				},
			},
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.K8sObject{
							Labels: map[string]string{
								"radius.dev/application": application,
							},
						},
					},
				},
			},
		},
	})
	test.Test(t)
}
