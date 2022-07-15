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

func Test_DaprStateStoreConnector(t *testing.T) {
	template := "testdata/connectorrp-resources-dapr-state-store.bicep"
	name := "connectorrp-resources-dapr-state-store"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "connectorrp-resources-dapr-state-store",
						Type: validation.ApplicationsResource,
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
