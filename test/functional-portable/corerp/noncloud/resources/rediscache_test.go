/*
Copyright 2026 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resource_test

import (
	"fmt"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
)

// Test_RedisCache deploys a Radius.Data/redisCaches resource using the default
// preview-environment recipe pack and validates the running Kubernetes resources
// provisioned by the Redis recipe.
func Test_RedisCache(t *testing.T) {
	template := "testdata/corerp-resources-rediscache.bicep"
	name := "corerp-resources-redis"
	cacheName := "redis-cache"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: cacheName,
						Type: validation.DataRedisCachesResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, cacheName),
						validation.NewK8sDeploymentForResource(name, cacheName),
						validation.NewK8sServiceForResource(name, cacheName),
					},
				},
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
