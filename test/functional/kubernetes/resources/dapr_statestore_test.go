// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStore_Generic(t *testing.T) {
	template := "testdata/kubernetes-resources-daprstatestore-generic.bicep"
	application := "kubernetes-resources-daprstatestore-generic"
	magpieImage := "magpieimage=" + os.Getenv("MAGPIE_IMAGE")
	fmt.Println("magpieImage:", magpieImage)
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template, magpieImage),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "myapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDeployment: validation.NewOutputResource(outputresource.LocalIDDeployment, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
					{
						ApplicationName: application,
						ResourceName:    "statestore",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDDaprStateStoreGeneric: validation.NewOutputResource(outputresource.LocalIDDaprStateStoreGeneric, outputresource.TypeKubernetes, resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(application, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
