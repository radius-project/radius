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

// Test_Container_Flatten deploys a container application whose bicep template
// uses the flattened authoring syntax (no .properties.{} envelope). It relies
// on x-ms-client-flatten support in the Radius Bicep type generator:
// fields such as environment, extensions, application, container, and
// connections are written directly at the resource level. The test passes only
// if (a) Bicep accepts the flat syntax against the regenerated types, and
// (b) the deployed resources behave identically to the legacy envelope form.
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
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, "ctnr-ctnr-flatten"),
					},
				},
			},
		},
	})

	test.Test(t)
}
