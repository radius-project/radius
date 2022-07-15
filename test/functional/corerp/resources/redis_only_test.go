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

func Test_RedisConnector(t *testing.T) {
	t.Skip("Will re-enable after: https://github.com/project-radius/deployment-engine/issues/146")

	template := "testdata/connectorrp-resources-redis-user-secrets.bicep"
	name := "connectorrp-resources-redis-user-secrets"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			Resources: []validation.Resource{
				{
					Name: "connectorrp-resources-redis-user-secrets",
					Type: validation.ApplicationsResource,
				},
				{
					Name: "redis",
					Type: validation.RedisCachesResource,
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
