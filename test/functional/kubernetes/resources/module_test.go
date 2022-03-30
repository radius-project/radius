// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
)

func TestK8sModule(t *testing.T) {
	template := "testdata/kubernetes-module/main.bicep"
	application := "kubernetes-module"
	magpieImage := "magpieimage=" + os.Getenv("MAGPIE_IMAGE")
	fmt.Println("magpieImage:", magpieImage)
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template, magpieImage),
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
					"default": {
						validation.NewK8sPodForResource(application, "container"),
						validation.NewK8sPodForResource(application, "busybox"),
					},
				},
			},
		},
	})
	test.Test(t)
}
