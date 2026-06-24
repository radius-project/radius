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
	"fmt"
	"testing"

	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_MultiCluster_BicepContainer verifies the Bicep engine path of multi-cluster v1.
//
// A Radius.Compute/containers resource is provisioned by a Bicep recipe executed
// by the Deployment Engine. When Radius is installed with the injected target
// kubeconfig, the Deployment Engine honors RADIUS_TARGET_KUBECONFIG and creates
// the container's Kubernetes resources on the external (workload) cluster rather
// than the control-plane cluster the Deployment Engine runs on.
//
// The test asserts the container pod exists on the external cluster and is absent
// from the control-plane cluster. It is skipped when RADIUS_TEST_EXTERNAL_KUBECONFIG
// is unset (single-cluster runs).
func Test_MultiCluster_BicepContainer(t *testing.T) {
	external := requireExternalCluster(t)

	template := "testdata/mcluster-resources-container.bicep"
	name := "mcluster-resources-container"
	containerName := "mcluster-ctnr"

	// The container pod lands in the environment's Kubernetes namespace (set to
	// the test name by the preview-env preSetup) on the external cluster.
	appNamespace := name

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
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

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}
