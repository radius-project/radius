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

// Test_TerraformContainer deploys a Radius.Compute/containers resource provisioned
// by a Terraform recipe (rather than the default Bicep recipe pack). It is the
// Terraform counterpart of Test_Container and the single-cluster base test that the
// multi-cluster Test_MultiCluster_TerraformContainer mirrors.
//
// The environment registers a custom recipe pack whose Radius.Compute/containers
// recipe is a Terraform module served by the in-cluster test module server. The
// environment namespace matches the test name, so the functional-test harness
// (CreateInitialResources) pre-creates it, satisfying the Radius.Core environment
// contract that the namespace must already exist.
func Test_TerraformContainer(t *testing.T) {
	template := "testdata/corerp-resources-container-terraform.bicep"
	name := "corerp-resources-container-tf"
	containerName := "ctnr-tf"
	appNamespace := name

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), testutil.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: containerName,
						Type: validation.ComputeContainersResource,
						App:  name,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					appNamespace: {
						validation.NewK8sPodForResource(name, containerName),
					},
				},
			},
		},
	})

	test.Test(t)
}
