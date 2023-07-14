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
	name := "corerp-resources-rabbitmq1"
	appNamespace := "default-corerp-resources-rabbitmq1"

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
						Name: "rmq-app-ctnr1",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rmq-ctnr1",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rmq-rte1",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "rmq-rmq1",
						Type: validation.RabbitMQMessageQueuesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-app-ctnr1"),
						validation.NewK8sPodForResource(name, "rmq-ctnr1"),
						validation.NewK8sServiceForResource(name, "rmq-rte1"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RabbitMQ_Recipe(t *testing.T) {
	template := "testdata/corerp-resources-rabbitmq-recipe.bicep"
	name := "corerp-resources-rabbitmq-recipe1"
	appNamespace := "default-corerp-resources-rabbitmq-recipe1"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "password=guest", functional.GetRecipeRegistry(), functional.GetRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-rabbitmq-recipe-env1",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rmq-recipe-app-ctnr1",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-recipe-app-ctnr1").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "rmq-recipe-resource1").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}
