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

func Test_MongoDBConnector(t *testing.T) {
	template := "testdata/connectorrp-resources-mongodb.bicep"
	name := "connectorrp-resources-mongodb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "connectorrp-resources-mongodb",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "db",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}

func Test_MongoDBConnectorUserSecrets(t *testing.T) {
	t.Skip()

	template := "testdata/connectorrp-resources-mongodb-user-secrets.bicep"
	name := "connectorrp-resources-mongodb-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "connectorrp-resources-mongodb-user-secrets",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mongo",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
