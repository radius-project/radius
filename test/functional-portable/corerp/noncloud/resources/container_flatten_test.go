/*
Copyright 2023 The Radius Authors.

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
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_Container_Flatten deploys an application with two container resources
// that exercise the read-side of x-ms-client-flatten support in the Radius
// Bicep type generator. Both resources are *authored* with the legacy
// `properties: { ... }` envelope (Radius RP only accepts that wire format),
// but the second container derives its fields from the first via the
// top-level read-only aliases the type generator now emits, e.g.
// `ctnr.container.image` and `ctnr.container.ports.web.containerPort` instead
// of `ctnr.properties.container.image` etc. If the generator regressed and
// stopped hoisting those aliases, Bicep compilation would fail and the deploy
// step would never reach the cluster.
func Test_Container_Flatten(t *testing.T) {
	template := "testdata/corerp-resources-container-flatten.bicep"
	name := "corerp-resources-container-flatten"
	appNamespace := "corerp-resources-container-flatten-app"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "ctnr-ctnr-flatten",
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: "ctnr-ctnr-flatten-2",
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-ctnr-flatten"),
						validation.NewK8sPodForResource(name, "ctnr-ctnr-flatten-2"),
					},
				},
			},
		},
	})

	test.Test(t)
}
