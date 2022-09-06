// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprPubSubGeneric(t *testing.T) {
	template := "testdata/corerp-resources-dapr-pubsub-generic.bicep"
	name := "corerp-resources-dapr-pubsub-generic"

	requiredSecrets := map[string]map[string]string{}

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
					"default": {
						validation.NewK8sPodForResource(name, "gnrc-publisher"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_DaprPubSubServiceBus(t *testing.T) {
	template := "testdata/corerp-resources-dapr-pubsub-servicebus.bicep"
	name := "corerp-resources-dapr-pubsub-servicebus"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
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
					"default": {
						validation.NewK8sPodForResource(name, "sb-publisher"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_DaprPubSubServiceInvalid(t *testing.T) {
	template := "testdata/corerp-resources-dapr-pubsub-servicebus-invalid.bicep"
	name := "corerp-resources-dapr-pubsub-servicebus-invalid"

	requiredSecrets := map[string]map[string]string{}

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
	}, requiredSecrets)

	test.Test(t)
}
