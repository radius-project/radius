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

	template := "testdata/corerp-resources-mongodb.bicep"
	name := "corerp-resources-mongodb"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-mongodb",
						Type: validation.ApplicationsResource,
					},
					{
						Name:    "webapp",
						Type:    validation.ContainersResource,
						AppName: "corerp-resources-mongodb",
					},
					{
						Name:    "db",
						Type:    validation.MongoDatabasesResource,
						AppName: "corerp-resources-mongodb",
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "webapp"),
					},
				},
			},
		},
	})

	test.Test(t)
}

func Test_MongoDBUserSecrets(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-user-secrets.bicep"
	name := "corerp-resources-mongodb-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-mongodb-user-secrets",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "app",
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
						Name: "mongo-db",
						Type: validation.MongoDatabasesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "app"),
						validation.NewK8sPodForResource(name, "mongo"),
					},
				},
			},
			SkipObjectValidation: false,
		},
	})

	test.Test(t)
}
