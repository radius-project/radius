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
	"fmt"
	"testing"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

// This file contains tests for general recipe engine functionality - covering general behaviors that should
// be consistent for all recipes and across all resource types. These tests use the extender resource
// and avoid a dependency on Bicep or Terraform drivers where possible to limit dependency coupling.
//
// See the recipe_bicep_test.go and recipe_terraform_test.go files for tests that cover driver-specific
// behaviors. Some functionality needs to be tested for each driver.

func Test_Recipe_NotFound(t *testing.T) {
	template := "testdata/corerp-resources-recipe-notfound.bicep"
	name := "corerp-resources-recipe-notfound"

	validate := step.ValidateSingleDetail("DeploymentFailed", step.DeploymentErrorDetail{
		Code: "ResourceDeploymentFailure",
		Details: []step.DeploymentErrorDetail{
			{
				Code:            recipes.RecipeNotFoundFailure,
				MessageContains: "could not find recipe \"not found!\" in environment",
			},
		},
	})

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, validate, fmt.Sprintf("basename=%s", name)),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.Test(t)
}
