// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"os"
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprPubSubGeneric(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-pubsub-generic.bicep"
	name := "corerp-resources-dapr-pubsub-generic"
	appNamespace := "default-corerp-resources-dapr-pubsub-generic"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-pubsub-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "gnrc-publisher",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "gnrc-pubsub",
						Type: validation.DaprPubSubBrokersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "gnrc-publisher"),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}

func Test_DaprPubSubServiceBus(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-pubsub-servicebus.bicep"
	name := "corerp-resources-dapr-pubsub-servicebus"

	if os.Getenv("AZURE_SERVICEBUS_RESOURCE_ID") == "" {
		t.Error("AZURE_SERVICEBUS_RESOURCE_ID environment variable must be set to run this test.")
	}
	namespaceresourceid := "namespaceresourceid=" + os.Getenv("AZURE_SERVICEBUS_RESOURCE_ID")
	appNamespace := "default-corerp-resources-dapr-pubsub-servicebus"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage(), namespaceresourceid),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-pubsub-servicebus",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "sb-publisher",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "sb-pubsub",
						Type: validation.DaprPubSubBrokersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "sb-publisher"),
					},
				},
			},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}

func Test_DaprPubSubServiceInvalid(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-pubsub-servicebus-invalid.bicep"
	name := "corerp-resources-dapr-pubsub-servicebus-invalid"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, v1.CodeInvalid, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-pubsub-servicebus-invalid",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}
