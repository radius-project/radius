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

func Test_RabbitMQ_Manual(t *testing.T) {
	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq-old"
	appNamespace := "default-corerp-resources-rabbitmq-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "password=guest"),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rmq-app-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rmq-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rmq-rte-old",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "rmq-rmq-old",
						Type: validation.RabbitMQMessageQueuesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-app-ctnr-old"),
						validation.NewK8sPodForResource(name, "rmq-ctnr-old"),
						validation.NewK8sServiceForResource(name, "rmq-rte-old"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RabbitMQ_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-rabbitmq-recipe.bicep"
	name := "corerp-resources-rabbitmq-recipe-old"
	appNamespace := "default-corerp-resources-rabbitmq-recipe-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "password=guest", functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-rabbitmq-recipe-env-old",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rmq-recipe-app-ctnr-old",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-recipe-app-ctnr-old").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "rmq-recipe-resource-old").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}
