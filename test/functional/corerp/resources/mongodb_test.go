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
//containerd://9eb0d205c4e34f9128c0a750f2854dc9ab09c6eab79679cbc924ccf75a09151d 0x14000b37d40}] BestEffort []}
func Test_MongoDB(t *testing.T) {
	t.Skip()
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
