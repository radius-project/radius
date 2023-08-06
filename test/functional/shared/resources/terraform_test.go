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

// Test_TerraformRecipe_HelloWorld covers the most basic possible terraform recipe scenario:
//
// - Create an extender resource using an empty Terraform recipe.
// - This way Terraform is executed, but no resources are created.
// - Since extender has no requirements on the Radius side, it will succeed.
func Test_TerraformRecipe_HelloWorld(t *testing.T) {
	template := "testdata/corerp-resources-terraform-helloworld.bicep"
	name := "corerp-resources-terraform-helloworld"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-terraform-helloworld",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-terraform-helloworld",
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects:           &validation.K8sObjectSet{},
			SkipResourceDeletion: true,
		},
	})
	test.Test(t)
}

func Test_TerraformRecipe_Context(t *testing.T) {
	template := "testdata/corerp-resources-terraform-context.bicep"
	name := "corerp-resources-terraform-context"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name,
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"corerp-resources-terraform-context-app": {
						validation.NewK8sSecretForResource(name, name),
					},
				},
			},
			SkipResourceDeletion: true,
		},
	})
	test.Test(t)
}
