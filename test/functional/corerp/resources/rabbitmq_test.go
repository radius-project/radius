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
	t.Skip()
	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq"

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
						Name: "webapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "rmq-ctr",
						Type: validation.ContainersResource,
					},
					{
						Name: "rmq-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "rmq",
						Type: validation.RabbitMQMessageQueuesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "webapp"),
						validation.NewK8sPodForResource(name, "rmq-ctr"),
						validation.NewK8sServiceForResource(name, "rmq-route"),
					},
				},
			},
		},
	})

	test.Test(t)
}
