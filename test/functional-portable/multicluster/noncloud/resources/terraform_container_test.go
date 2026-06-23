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
	"context"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_MultiCluster_TerraformContainer verifies the Terraform path of multi-cluster
// v1 for the new Radius.Compute/containers type. It is the multi-cluster mirror of
// the single-cluster Test_TerraformContainer.
//
// A Radius.Compute/containers resource is provisioned by a Terraform recipe (served
// by the in-cluster test module server) registered through a custom recipe pack.
// Terraform executes on the control plane, but its Kubernetes provider honors the
// injected target kubeconfig (via the cluster access resolver), so the container's
// Kubernetes resources are created on the external (workload) cluster rather than
// the control-plane cluster.
//
// The test asserts the container pod exists on the external cluster and is absent
// from the control-plane cluster. It is skipped when RADIUS_TEST_EXTERNAL_KUBECONFIG
// is unset (single-cluster runs).
func Test_MultiCluster_TerraformContainer(t *testing.T) {
	external := requireExternalCluster(t)

	template := "testdata/mcluster-resources-container-terraform.bicep"
	name := "mcluster-resources-container-tf"
	containerName := "mcluster-tf-ctnr"

	// The container pod lands in the environment's Kubernetes namespace (set to the
	// test name, which the harness pre-creates on the external cluster) on the
	// external cluster.
	appNamespace := name

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage(), testutil.GetTerraformRecipeModuleServerURL()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "mcluster-resources-container-tf-pack",
						Type: "radius.core/recipepacks",
					},
					{
						Name: "mcluster-resources-container-tf-env",
						Type: validation.CoreEnvironmentsResource,
					},
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
			// The control-plane cluster has none of the recipe-created workload, so
			// the default K8sObjects validation (which runs against the control-plane
			// cluster) is intentionally empty. External-cluster assertions happen in
			// PostStepVerify below.
			K8sObjects: &validation.K8sObjectSet{},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// The container pod must exist on the external (workload) cluster.
				validation.ValidateObjectsRunning(ctx, t, external.clientset, external.dynamicClient, validation.K8sObjectSet{
					Namespaces: map[string][]validation.K8sObject{
						appNamespace: {
							validation.NewK8sPodForResource(name, containerName),
						},
					},
				})

				// The container pod must NOT exist on the control-plane cluster.
				requireNoPodsForResource(ctx, t, test.Options.K8sClient, appNamespace, name, containerName)
			},
		},
	})

	test.Test(t)
}
