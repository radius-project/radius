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

func Test_SQLDatabase(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-sqldb.bicep"
	name := "corerp-resources-sqldb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewTempCoreRPExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-sqldb-app",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "todoapp",
					Type: validation.ContainersResource,
				},
				{
					Name: "db",
					Type: validation.SqlDatabase,
				},
			},
		},
	})

	test.Test(t)
}
