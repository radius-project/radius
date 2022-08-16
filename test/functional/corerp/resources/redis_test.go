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

func Test_Redis(t *testing.T) {
	template := "testdata/corerp-resources-redis-user-secrets.bicep"
	name := "corerp-resources-redis-user-secrets"

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
						Name: "rds-app-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-ctnr",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "rds-rte",
						Type: validation.HttpRoutesResource,
						App:  name,
					},
					{
						Name: "rds-rds",
						Type: validation.RedisCachesResource,
						App:  name,
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
	}, requiredSecrets)

	test.Test(t)
}
