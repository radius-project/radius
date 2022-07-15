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

func Test_DaprSecretStoreConnector(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/connectorrp-resources-dapr-secret-store.bicep"
	name := "connectorrp-resources-dapr-secret-store"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "connectorrp-resources-dapr-secret-store",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "secretstore",
					Type: validation.DaprSecretStoresResource,
				},
			},
		},
	})

	test.Test(t)
}
