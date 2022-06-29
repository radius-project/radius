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

func Test_Redis(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-redis-user-secrets.bicep"
	name := "corerp-resources-redis-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewTempCoreRPExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-redis-user-secrets",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "todoapp",
					Type: validation.ContainersResource,
				},
				{
					Name: "redis",
					Type: validation.ContainersResource,
				},
				{
					Name: "redis-route",
					Type: validation.HttpRoutesResource,
				},
				{
					Name: "redis",
					Type: validation.RedisCachesResource,
				},
			},
		},
	})

	test.Test(t)
}
