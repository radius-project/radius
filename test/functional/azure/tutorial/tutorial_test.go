// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package tutorial_test

import (
	"testing"

	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_TutorialDaprMicroservices(t *testing.T) {
	application := "dapr-hello"
	template := "../../../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-hello": {
						validation.NewK8sObjectForComponent("dapr-hello", "nodeapp"),
						validation.NewK8sObjectForComponent("dapr-hello", "pythonapp"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	test.Test(t)
}
