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

// TODO: webapp logs this error:
// failed to connect with redis instance at corerp-resources-redis-user-secrets-redis-route:80 -
// dial tcp 10.96.251.170:80: connect: connection refused
func Test_Redis(t *testing.T) {
	template := "testdata/corerp-resources-redis-user-secrets.bicep"
	name := "corerp-resources-redis-user-secrets"

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
						Name: "rds-app-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "rds-ctnr",
						Type: validation.ContainersResource,
					},
					{
						Name: "rds-rte",
						Type: validation.HttpRoutesResource,
					},
					{
						Name: "rds-rds",
						Type: validation.RedisCachesResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"default": {
						validation.NewK8sPodForResource(name, "rds-app-ctnr"),
						validation.NewK8sPodForResource(name, "rds-ctnr"),
						validation.NewK8sServiceForResource(name, "rds-rte"),
					},
				},
			},
		},
	})

	test.Test(t)
}
