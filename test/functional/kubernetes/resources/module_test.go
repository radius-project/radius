// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/executor"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/kubernetes"
	"github.com/project-radius/radius/test/validation"
)

func TestK8sModule(t *testing.T) {
	template := "testdata/kubernetes-module/main.bicep"
	application := "kubernetes-module"

	test := kubernetes.NewApplicationTest(t, application, []kubernetes.Step{
		{
			Executor: executor.NewDeployStepExecutor(template, functional.GetMagpieImage()),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "application",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sPodForResource(application, "container"),
						validation.NewK8sPodForResource(application, "busybox"),
					},
				},
			},
		},
	})
	test.Test(t)
}
