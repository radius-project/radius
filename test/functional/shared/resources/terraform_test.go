/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// Test_TerraformRecipe_Redis covers the following terraform recipe scenario:
//
// - Create an extender resource using a Terraform recipe that deploys Redis on Kubernetes.
// - The recipe deployment creates a Kubernetes deployment and a Kubernetes service.
func Test_TerraformRecipe_Redis(t *testing.T) {
	template := "testdata/corerp-resources-terraform-redis.bicep"
	name := "corerp-resources-terraform-redis"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-terraform-redis-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-terraform-redis-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name:            "corerp-resources-terraform-redis",
						Type:            validation.ExtendersResource,
						App:             "corerp-resources-terraform-redis-app",
						OutputResources: []validation.OutputResourceResponse{}, // No output resources because Terraform Recipe outputs aren't integreted yet.
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sServiceForResource(name, "redis-cache-tf-recipe"),
					},
				},
			},
			SkipResourceDeletion: true, // Skip deletion because Terraform Recipe deletion isn't supported yet.
		},
	})
	test.Test(t)
}
