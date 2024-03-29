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

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_Extender_Manual(t *testing.T) {
	template := "testdata/corerp-resources-extender.bicep"
	name := "corerp-resources-extender"
	appNamespace := "default-corerp-resources-extender"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "extr-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "extr-twilio",
						Type: validation.ExtendersResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "extr-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_Extender_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-extender-recipe.bicep"
	name := "corerp-resources-extender-recipe"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-extender-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "extender-recipe",
						Type: validation.ExtendersResource,
						App:  name,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
