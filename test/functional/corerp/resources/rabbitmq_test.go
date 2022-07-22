// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// TODO: webapp logs this error:
// 2022/07/16 20:44:18 Failed to connect to RabbitMQ -  dial tcp 10.96.187.212:5672: connect: connection refused
// 2022/07/16 20:44:25 Failed to connect to RabbitMQ -  dial tcp 10.96.187.212:5672: connect: connection refused
func Test_RabbitMQ(t *testing.T) {
	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "rmq-app-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "rmq-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "rmq-rte",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "rmq-rmq",
						Type: validation.RabbitMQMessageQueuesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "rmq-app-ctnr"),
						validation.NewK8sPodForResource(name, "rmq-ctnr"),
						validation.NewK8sServiceForResource(name, "rmq-rte"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
