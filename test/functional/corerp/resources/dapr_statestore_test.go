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

func Test_DaprStateStoreGeneric(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

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
		},
	})

	test.Test(t)
}

func Test_DaprStateStoreTableStorage(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

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
		},
	})

	test.Test(t)
}
