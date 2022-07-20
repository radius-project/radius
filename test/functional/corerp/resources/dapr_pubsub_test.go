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
						Name: "gnrc-publisher",
						Type: validation.ContainersResource,
					},
					{
						Name: "gnrc-pubsub",
						Type: validation.DaprPubSubResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "gnrc-publisher"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_DaprPubSubServiceBus(t *testing.T) {
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
						Name: "sb-publisher",
						Type: validation.ContainersResource,
					},
					{
						Name: "sb-pubsub",
						Type: validation.DaprPubSubResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "sb-publisher"),
					},
				},
			},
		},
	})

	test.Test(t)
}
