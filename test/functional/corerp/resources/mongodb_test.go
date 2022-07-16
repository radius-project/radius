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

// TODO: Getting "Unauthorized" error
// Error: Code="DeploymentFailed" Message="" Details=[{"additionalInfo":null,"code":"OK","details":null,"message":"","target":null},
// {"additionalInfo":null,"code":"Unauthorized","details":null,"message":"{\n  \"error\": {\n    \"code\": \"AuthenticationFailed\",\n
// \"message\": \"Authentication failed. The 'Authorization' header is missing.\"\n  }\n}","target":null}]
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
						Name: name,
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
						validation.NewK8sServiceForResource(name, "mongo-route"),
					},
				},
			},
		},
	})

	test.Test(t)
}
