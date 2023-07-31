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
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprStateStore_Manual(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-manual.bicep"
	name := "corerp-resources-dsstm-old"
	appNamespace := "default-corerp-resources-dsstm-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), fmt.Sprintf("namespace=%s", appNamespace)),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-dsstm-old",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "dapr-sts-manual-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-sts-manual-old",
						Type: validation.O_DaprStateStoresResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dapr-sts-manual-ctnr-old"),

						// Deployed as supporting resources using Kubernetes Bicep extensibility.
						validation.NewK8sPodForResource(name, "dapr-sts-manual-redis-old").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "dapr-sts-manual-redis-old").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}

func Test_DaprStateStore_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-dapr-statestore-recipe.bicep"
	name := "corerp-rs-dapr-sts-recipe-old"
	appNamespace := "corerp-env-recipes-env-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-env-recipes-env-old",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-rs-dapr-sts-recipe-old",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "dapr-sts-recipe-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dapr-sts-recipe-old",
						Type: validation.O_DaprStateStoresResource,
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
						validation.NewK8sPodForResource(name, "dapr-sts-recipe-ctnr-old").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}
