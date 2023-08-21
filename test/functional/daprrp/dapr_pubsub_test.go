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

	"github.com/radius-project/radius/pkg/resourcemodel"
	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_DaprPubSubBroker_Manual(t *testing.T) {
	template := "resources/testdata/daprrp-resources-pubsub-broker-manual.bicep"
	name := "dpsb-manual-app"
	appNamespace := "default-dpsb-manual-app"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), fmt.Sprintf("namespace=%s", appNamespace)),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dpsb-manual-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "dpsb-manual-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dpsb-manual",
						Type: validation.DaprPubSubBrokersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dpsb-manual-app-ctnr"),
						validation.NewK8sPodForResource(name, "dpsb-manual-redis").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "dpsb-manual-redis").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}

func Test_DaprPubSubBroker_Recipe(t *testing.T) {
	template := "resources/testdata/daprrp-resources-pubsub-broker-recipe.bicep"
	name := "dpsb-recipe-app"
	appNamespace := "dpsb-recipe-env"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dpsb-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dpsb-recipe-app",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "dpsb-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dpsb-recipe",
						Type: validation.DaprPubSubBrokersResource,
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
						validation.NewK8sPodForResource(name, "dpsb-recipe-ctnr").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}
