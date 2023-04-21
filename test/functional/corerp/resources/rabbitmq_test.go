// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_RabbitMQ(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-rabbitmq.bicep"
	name := "corerp-resources-rabbitmq"
	appNamespace := "default-corerp-resources-rabbitmq"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), "password=guest"),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
						Name: "rmq-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "rmq-rmq",
						Type: validation.RabbitMQMessageQueuesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "rmq-app-ctnr"),
						validation.NewK8sPodForResource(name, "rmq-ctnr"),
						validation.NewK8sServiceForResource(name, "rmq-rte"),
					},
				},
			},
		},
	})

	test.Test(t)
}
