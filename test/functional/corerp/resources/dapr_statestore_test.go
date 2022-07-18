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

// FIXME: Getting this error:
// failed to create Dapr client -  error creating default client: error creating connection
// to '127.0.0.1:50001': context deadline exceeded: context deadline exceeded.
// Probably missing something in the container declaration in bicep file.
func Test_DaprStateStoreGeneric(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-statestore-generic.bicep"
	name := "corerp-resources-dapr-statestore-generic"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-statestore-generic",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "myapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "statestore-generic",
						Type: validation.DaprStateStoreResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

// FIXME: Getting this error:
// 2022/07/19 02:46:38 failed to create Dapr client -  error creating default client: error creating connection to '127.0.0.1:50001': context deadline exceeded: context deadline exceeded
// 2022/07/19 02:46:38 http: panic serving 10.244.0.1:39234: runtime error: invalid memory address or nil pointer dereference
func Test_DaprStateStoreTableStorage(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-dapr-statestore-tablestorage.bicep"
	name := "corerp-resources-dapr-statestore-tablestorage"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),

			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-statestore-tablestorage",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "myapp",
						Type: validation.ContainersResource,
					},
					{
						Name: "mystore",
						Type: validation.DaprStateStoreResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "myapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}
