// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	"github.com/project-radius/radius/pkg/radrp/outputresource"
	"github.com/project-radius/radius/pkg/radrp/rest"
	"github.com/project-radius/radius/pkg/resourcekinds"
	"github.com/project-radius/radius/test/kubernetestest"
	"github.com/project-radius/radius/test/validation"
)

func TestRabbitMQ(t *testing.T) {
	template := "testdata/kubernetes-resources-rabbitmq/kubernetes-resources-rabbitmq.bicep"
	application := "kubernetes-resources-rabbitmq"
	magpieImage := "magpieimage=" + os.Getenv("MAGPIE_IMAGE")

	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template, magpieImage),
			RadiusResources: &validation.ResourceSet{
				Resources: []validation.RadiusResource{
					{
						ApplicationName: application,
						ResourceName:    "todoapp",
						OutputResources: map[string]validation.ExpectedOutputResource{
							outputresource.LocalIDScrapedSecret: validation.NewOutputResource(
								outputresource.LocalIDScrapedSecret,
								outputresource.TypeKubernetes,
								resourcekinds.Kubernetes, false, rest.OutputResourceStatus{}),
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(application, "todoapp"),
					},
				},
			},
		},
	}, loadResources("testdata/kubernetes-resources-rabbitmq", ".input.yaml")...)

	test.Test(t)
}
