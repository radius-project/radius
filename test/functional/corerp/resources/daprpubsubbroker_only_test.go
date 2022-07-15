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
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/connectorrp-resources-dapr-pubsub-broker.bicep"
	name := "connectorrp-resources-dapr-pubsub-broker"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "connectorrp-resources-dapr-pubsub-broker",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "pubsubbbroker",
					Type: validation.DaprPubSubBrokerResource,
				},
			},
		},
	})

	test.Test(t)
}
