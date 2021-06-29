// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resources_test

import (
	"testing"

	"github.com/Azure/radius/test/azuretest"
	"github.com/Azure/radius/test/validation"
)

func Test_ServiceBusManaged(t *testing.T) {
	application := "azure-resources-servicebus-managed"
	template := "testdata/azure-resources-servicebus-managed.bicep"
	test := azuretest.NewApplicationTest(t, application, []azuretest.Step{
		{
			Executor: azuretest.NewDeployStepExecutor(template),
			Pods: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					application: {
						validation.NewK8sObjectForComponent(application, "sender"),
						validation.NewK8sObjectForComponent(application, "receiver"),
					},
				},
			},
			SkipARMResources: true,
			SkipComponents:   true,
		},
	})

	test.Test(t)
}
