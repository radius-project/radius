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
	"github.com/radius-project/radius/test/validation"
)

func Test_ACI(t *testing.T) {
	name := "aci-app"
	containerResourceName := "frontend"
	containerResourceName2 := "magpie"
	gatewayResourceName := "gateway"
	template := "testdata/corerp-aci.bicep"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor:             step.NewDeployExecutor(template),
			SkipObjectValidation: true,
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerResourceName,
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: containerResourceName2,
						Type: validation.ContainersResource,
						App:  name,
					},
					{
						Name: gatewayResourceName,
						Type: validation.GatewaysResource,
						App:  name,
					},
				},
			},
		},
	})

	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.Test(t)
}
