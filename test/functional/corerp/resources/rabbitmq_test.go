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
						Name: "corerp-resources-rabbitmq-app",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "corerp-resources-rabbitmq-webapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-rabbitmq-container",
						Type: validation.ContainersResource,
					},
					{
						Name: "corerp-resources-rabbitmq-route",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "corerp-resources-rabbitmq-rabbitmq",
						Type: validation.RabbitMQMessageQueuesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "corerp-resources-rabbitmq-webapp"),
						validation.NewK8sPodForResource(name, "corerp-resources-rabbitmq-container"),
						validation.NewK8sHTTPProxyForResource(name, "corerp-resources-rabbitmq-route"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
