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

func TestRedis(t *testing.T) {
	template := "testdata/kubernetes-resources-redis/kubernetes-resources-redis.bicep"
	application := "kubernetes-resources-redis"
	test := kubernetestest.NewApplicationTest(t, application, []kubernetestest.Step{
		{
			Executor: kubernetestest.NewDeployStepExecutor(template),
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
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						// This will ensure that all the connection-properties
						// were populated correctly.
						validation.NewK8sObjectForResource(application, "todoapp"),
					},
				},
			},
		},
	}, loadResources("testdata/kubernetes-resources-redis", ".input.yaml")...)

	test.Test(t)
}
