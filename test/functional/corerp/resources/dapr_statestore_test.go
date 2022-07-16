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

// TODO: Getting "Unauthorized" error
// Error: Code="DeploymentFailed" Message="" Details=[{"additionalInfo":null,"code":"OK","details":null,"message":"","target":null},
// {"additionalInfo":null,"code":"Unauthorized","details":null,"message":"{\n  \"error\": {\n    \"code\": \"AuthenticationFailed\",\n
// \"message\": \"Authentication failed. The 'Authorization' header is missing.\"\n  }\n}","target":null}]
func Test_DaprStateStoreTableStorage(t *testing.T) {
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
