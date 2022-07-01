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

func Test_MongoDB(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-mongodb.bicep"
	name := "corerp-resources-mongodb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-mongodb-app",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "todoapp",
					Type: validation.ContainersResource,
				},
				{
					Name: "db",
					Type: validation.MongoDatabasesResource,
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDBUserSecrets(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/corerp-resources-mongodb-user-secrets.bicep"
	name := "corerp-resources-mongodb-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "corerp-resources-mongodb-user-secrets-app",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "todoapp",
					Type: validation.ContainersResource,
				},
				{
					Name: "mongo",
					Type: validation.ContainersResource,
				},
				{
					Name: "mongo-route",
					Type: validation.HttpRoutesResource,
				},
				{
					Name: "mongo",
					Type: validation.MongoDatabasesResource,
				},
			},
		},
	})

	test.Test(t)
}
