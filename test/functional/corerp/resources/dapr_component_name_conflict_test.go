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

	v1 "github.com/project-radius/radius/pkg/armrpc/api/v1"
	"github.com/project-radius/radius/test/functional/corerp"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprComponentNameConflict(t *testing.T) {
	template := "testdata/corerp-resources-dapr-component-name-conflict.bicep"
	name := "corerp-resources-dapr-component-name-conflict"

	test := corerp.NewCoreRPTest(t, name, []corerp.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, v1.CodeInternal),
			CoreRPResources: &validation.CoreRPResourceSet{
				Resources: []validation.CoreRPResource{
					{
						Name: "corerp-resources-dapr-component-name-conflict",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.RequiredFeatures = []corerp.RequiredFeature{corerp.FeatureDapr}

	test.Test(t)
}
