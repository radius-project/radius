// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package resource_test

import (
	"testing"

	"github.com/project-radius/radius/test/functional"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

// FIXME: Logs:
// watching pod corerp-resources-mongodb-webapp-5844b958f5-tsdvd for status.. current:
// {Running [{Initialized True 0001-01-01 00:00:00 +0000 UTC 2022-07-18 19:52:53 -0700 PDT  }
// {Ready False 0001-01-01 00:00:00 +0000 UTC 2022-07-18 19:52:53 -0700 PDT ContainersNotReady
// containers with unready status: [webapp]} {ContainersReady False 0001-01-01 00:00:00 +0000 UTC 2022-07-18
// 19:52:53 -0700 PDT ContainersNotReady containers with unready status: [webapp]}
// {PodScheduled True 0001-01-01 00:00:00 +0000 UTC 2022-07-18 19:52:53 -0700 PDT  }]
// 172.18.0.2 10.244.0.51 [{10.244.0.51}] 2022-07-18 19:52:53 -0700 PDT [] [{webapp
// {nil &ContainerStateRunning{StartedAt:2022-07-18 19:52:54 -0700 PDT,} nil} {nil nil nil} false 0
// radiusdev.azurecr.io/magpiego:latest radiusdev.azurecr.io/magpiego@sha256:b11165040c3ca4b63d836fb68ae3511b6b19f9e826a86fb4be7de1afffaf8a5f
// containerd://9eb0d205c4e34f9128c0a750f2854dc9ab09c6eab79679cbc924ccf75a09151d 0x14000b37d40}] BestEffort []}
func Test_MongoDB(t *testing.T) {
	t.Skip()
	template := "testdata/corerp-resources-mongodb.bicep"
	name := "corerp-resources-mongodb"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-mongodb",
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mdb-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mdb-app-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

func Test_MongoDBUserSecrets(t *testing.T) {
	template := "testdata/corerp-resources-mongodb-user-secrets.bicep"
	name := "corerp-resources-mongodb-user-secrets"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "mdb-us-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mdb-us-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "mdb-us-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "mdb-us-app-ctnr"),
						validation.NewK8sPodForResource(name, "mdb-us-ctnr"),
						validation.NewK8sServiceForResource(name, "mdb-us-rte"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}

// Test_MongoDB_Recipe validates:
// the creation of a mongoDB from recipe
// container using the mongoDB connector to connect to the mongoDB resource
func Test_MongoDB_Recipe(t *testing.T) {

	template := "testdata/corerp-resources-mongodb-recipe.bicep"
	name := "corerp-resources-mongodb-recipe"

	requiredSecrets := map[string]map[string]string{}

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, functional.GetMagpieImage()),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-environment-recipes-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: "corerp-resources-mongodb-recipe",
						Type: validation.ApplicationsResource,
						App:  name,
					},
					{
						Name: "mongodb-recipe-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "mongo-recipe-db",
						Type: validation.MongoDatabasesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"corerp-resources-environment-recipes-env": {
						validation.NewK8sPodForResource(name, "mongodb-recipe-app-ctnr"),
					},
				},
			},
		},
	}, requiredSecrets)

	test.Test(t)
}
