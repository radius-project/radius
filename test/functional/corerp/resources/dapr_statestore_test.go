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

	"github.com/project-radius/radius/pkg/resourcemodel"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStore_Manual(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-manual.bicep"
	name := "corerp-resources-dapr-statestore-manual"
	appNamespace := "default-corerp-resources-dapr-statestore-manual"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), fmt.Sprintf("namespace=%s", appNamespace)),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-statestore-manual",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "dapr-sts-manual-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-sts-manual",
						Type: validation.DaprStateStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dapr-sts-manual-ctnr"),

						// Deployed as supporting resources using Kubernetes Bicep extensibility.
						validation.NewK8sPodForResource(name, "dapr-sts-manual-redis").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "dapr-sts-manual-redis").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}
	test.Test(t)
}

func Test_DaprStateStore_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-recipe.bicep"
	name := "corerp-resources-dapr-sts-recipe"
	appNamespace := "corerp-environment-recipes-env"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-environment-recipes-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-dapr-sts-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "dapr-sts-recipe-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-sts-recipe",
						Type: validation.DaprStateStoresResource,
						App:  name,
						OutputResources: []validation.OutputResourceResponse{
							{
								Provider: resourcemodel.ProviderKubernetes,
								LocalID:  "RecipeResource0",
							},
							{
								Provider: resourcemodel.ProviderKubernetes,
								LocalID:  "RecipeResource1",
							},
						},
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dapr-sts-recipe-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}
	test.Test(t)
}
