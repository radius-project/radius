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
	"github.com/radius-project/radius/test/validation"
)

// Test_MultiCluster_LegacyContainer verifies the direct-resource path of
// multi-cluster v1.
//
// A legacy Applications.Core/containers resource is rendered directly by
// applications-rp (the Go renderer chain, not a recipe). When Radius is installed
// with the injected target kubeconfig, the applications-rp async worker applies
// the rendered Deployment to the external (workload) cluster instead of the
// control-plane cluster it runs on.
//
// The test asserts the container pod exists on the external cluster and is absent
// from the control-plane cluster. It is skipped when RADIUS_TEST_EXTERNAL_KUBECONFIG
// is unset (single-cluster runs).
func Test_MultiCluster_LegacyContainer(t *testing.T) {
	external := requireExternalCluster(t)

	template := "testdata/mcluster-resources-legacy-container.bicep"
	name := "mcluster-resources-legacy-container"
	containerName := "mcluster-legacy-ctnr"
	appNamespace := "mcluster-resources-legacy-container-app"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.ApplicationsResource,
					},
					{
						Name: containerName,
						Type: validation.ContainersResource,
						App:  name,
					},
				},
			},
			// The control-plane cluster has none of the workload, so the default
			// K8sObjects validation (which runs against the control-plane cluster) is
			// intentionally empty. External-cluster assertions happen in PostStepVerify.
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
