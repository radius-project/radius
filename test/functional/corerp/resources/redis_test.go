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
	t.Skip()

	template := "testdata/corerp-resources-redis-user-secrets.bicep"
	name := "corerp-resources-redis-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
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
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, "webapp"),
						validation.NewK8sPodForResource(name, "redis"),
						validation.NewK8sHTTPProxyForResource(name, "redis-route"),
					},
				},
			},
			SkipObjectValidation: true,
		},
	})

	test.Test(t)
}
