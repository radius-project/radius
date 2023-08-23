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

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprPubSubBroker_Manual(t *testing.T) {
	template := "testdata/corerp-resources-dapr-pubsub-broker-manual.bicep"
	name := "dpsb-mnl-app-old"
	appNamespace := "default-dpsb-mnl-app-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), fmt.Sprintf("namespace=%s", appNamespace)),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dpsb-mnl-app-old",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "dpsb-mnl-app-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dpsb-mnl-old",
						Type: validation.O_DaprPubSubBrokersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dpsb-mnl-app-ctnr-old"),
						validation.NewK8sPodForResource(name, "dpsb-mnl-redis-old").ValidateLabels(false),
						validation.NewK8sServiceForResource(name, "dpsb-mnl-redis-old").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}

func Test_DaprPubSubBroker_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-dapr-pubsub-broker-recipe.bicep"
	name := "dpsb-recipe-app-old"
	appNamespace := "dpsb-recipe-env-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "dpsb-recipe-env-old",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "dpsb-recipe-app-old",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "dpsb-recipe-app-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "dpsb-recipe-old",
						Type: validation.O_DaprPubSubBrokersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "dpsb-recipe-app-ctnr-old").ValidateLabels(false),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}
	test.Test(t)
}
