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

func Test_RabbitMQConnector(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/connectorrp-resources-rabbitmq.bicep"
	name := "connectorrp-resources-rabbitmq"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "connectorrp-resources-rabbitmq",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "rabbitmq",
					Type: validation.RabbitMQResource,
				},
			},
		},
	})

	test.Test(t)
}
