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

func Test_DaprPubSubGeneric(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-dapr-pubsub-generic.bicep"
	name := "corerp-resources-dapr-pubsub-generic"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-pubsub-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "publisher",
						Type: validation.ContainersResource,
					},
					{
						Name: "pubsub",
						Type: validation.DaprPubSubResource,
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_DaprPubSubServiceBus(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-dapr-pubsub-servicebus.bicep"
	name := "corerp-resources-dapr-pubsub-servicebus"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),

			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-pubsub-servicebus",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "publisher",
						Type: validation.ContainersResource,
					},
					{
						Name: "pubsub",
						Type: validation.DaprPubSubResource,
					},
				},
			},
		},
	})

	test.Test(t)
}
