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

	"github.com/radius-project/radius/test/functional"
	"github.com/radius-project/radius/test/functional/shared"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

func Test_RabbitMQ_Manual(t *testing.T) {
	template := "testdata/msgrp-resources-rabbitmq.bicep"
	name := "msgrp-resources-rabbitmq"
	appNamespace := "default-msgrp-resources-rabbitmq"

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
						Name: "rmq-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rmq-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "msg-rmq-rmq",
						Type: validation.RabbitMQQueuesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-app-ctnr"),
						validation.NewK8sPodForResource(name, "rmq-ctnr"),
						validation.NewK8sServiceForResource(name, "rmq-ctnr"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_RabbitMQ_Recipe(t *testing.T) {
	template := "testdata/msgrp-resources-rabbitmq-recipe.bicep"
	name := "msgrp-resources-rabbitmq-recipe"
	appNamespace := "default-msgrp-resources-rabbitmq-recipe"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "password=guest", functional.GetBicepRecipeRegistry(), functional.GetBicepRecipeVersion()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "msgrp-resources-environment-rabbitmq-recipe-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rmq-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-recipe-app-ctnr").ValidateLabels(false),
						validation.NewK8sPodForResource(name, "rmq-recipe-resource").ValidateLabels(false),
					},
				},
			},
		},
	})

	test.Test(t)
}
