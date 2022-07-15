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

func Test_DaprPubSubBrokeConnector(t *testing.T) {
	template := "testdata/connectorrp-resources-dapr-pubsub-broker.bicep"
	name := "connectorrp-resources-dapr-pubsub-broker"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "connectorrp-resources-dapr-pubsub-broker",
						Type: validation.ApplicationsResource,
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
