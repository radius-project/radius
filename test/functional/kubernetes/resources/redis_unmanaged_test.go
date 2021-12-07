// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/Azure/radius/pkg/radrp/outputresource"
	"github.com/Azure/radius/pkg/radrp/rest"
	"github.com/Azure/radius/pkg/resourcekinds"
	"github.com/Azure/radius/test/kubernetestest"
	"github.com/Azure/radius/test/validation"
)

func TestRedisUnmanaged(t *testing.T) {
	t.Skip("Disable until we reinstate full-references for Bicep")
	template := "testdata/kubernetes-resources-redis-unmanaged/kubernetes-resources-redis-unmanaged.bicep"
	application := "kubernetes-resources-redis-unmanaged"
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
								resourcekinds.Kubernetes, true, false, rest.OutputResourceStatus{}),
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
	}, loadResources("testdata/kubernetes-resources-redis-unmanaged", ".input.yaml")...)

	test.Test(t)
}
