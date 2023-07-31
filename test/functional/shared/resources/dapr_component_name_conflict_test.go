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
	"github.com/project-radius/radius/test/functional/shared"
	"github.com/project-radius/radius/test/step"
	"github.com/project-radius/radius/test/validation"
)

func Test_DaprComponentNameConflict(t *testing.T) {
	template := "testdata/corerp-resources-dapr-component-name-conflict.bicep"
	name := "corerp-resources-dcnc-old"

	test := shared.NewRPTest(t, name, []shared.TestStep{
		{
			Executor: step.NewDeployErrorExecutor(template, v1.CodeInternal, nil),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-dcnc-old",
						Type: validation.ApplicationsResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{},
		},
	})
	test.RequiredFeatures = []shared.RequiredFeature{shared.FeatureDapr}

	test.Test(t)
}
