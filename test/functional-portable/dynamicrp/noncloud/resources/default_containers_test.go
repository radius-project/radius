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

// Test_DefaultContainers_Deploy verifies that the default Radius.Compute/containers
// and Radius.Compute/routes resource types (copied from resource-types-contrib)
// can be deployed end-to-end.
//
// The types and their recipes are registered at startup from the copied manifests
// and the default recipe pack. This test validates the full path:
//   - Manifests loaded at startup (from built-in-providers/)
//   - Types registered in UCP (via registerResourceProviderDirect)
//   - Default recipes available (from recipe pack)
//   - Resources deployed successfully via recipes
//
// Using two types from the same namespace (Radius.Compute) also validates that
// the namespace merge fix works correctly in a real deployment scenario.
func Test_DefaultContainers_Deploy(t *testing.T) {
	template := "testdata/default-containers-test.bicep"
	appName := "default-containers-app"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: appName,
						Type: "Radius.Core/applications",
					},
					{
						Name: "default-container",
						Type: "Radius.Compute/containers",
					},
					{
						Name: "default-route",
						Type: "Radius.Compute/routes",
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
		},
	})

	test.Test(t)
}
