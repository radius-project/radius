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

	"github.com/stretchr/testify/require"

	"github.com/radius-project/radius/test/functional-portable/corerp"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

// Test_MultiCluster_TerraformRecipe verifies the Terraform path of multi-cluster
// v1 and that the Terraform state backend stays on the control-plane cluster.
//
// A Terraform recipe deploys a Redis Deployment and Service on Kubernetes. When
// Radius is installed with the injected target kubeconfig, the Terraform
// kubernetes provider (via the cluster access resolver) creates those resources
// on the external (workload) cluster, while the Terraform state secret continues
// to be written to the control-plane cluster (radius-system).
//
// The test asserts the Redis service exists on the external cluster and is absent
// on the control plane, and that the tfstate secret exists on the control plane.
// It is skipped when RADIUS_TEST_EXTERNAL_KUBECONFIG is unset.
func Test_MultiCluster_TerraformRecipe(t *testing.T) {
	external := requireExternalCluster(t)

	template := "testdata/mcluster-resources-terraform-redis.bicep"
	name := "mcluster-resources-terraform-redis"
	appName := "mcluster-resources-terraform-redis-app"
	envName := "mcluster-resources-terraform-redis-env"
	redisCacheName := "tf-redis-cache"

	secretNamespace := "radius-system"
	secretPrefix := "tfstate-default-"
	secretSuffix, err := corerp.GetSecretSuffix(
		"/planes/radius/local/resourcegroups/kind-radius/providers/Applications.Core/extenders/"+name,
		envName, appName)
	require.NoError(t, err)

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetTerraformRecipeModuleServerURL(),
				"appName="+appName, "envName="+envName, "resourceName="+name, "redisCacheName="+redisCacheName),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: envName,
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: name,
						Type: validation.ExtendersResource,
						App:  appName,
					},
				},
			},
			// The Terraform state secret stays on the control-plane cluster, so
			// validate it there via the default K8sObjects assertion. The recipe's
			// workload resources are validated on the external cluster below.
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					secretNamespace: {
						validation.NewK8sSecretForResourceWithResourceName(secretPrefix + secretSuffix).
							ValidateLabels(false),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				// The Redis service must exist on the external (workload) cluster.
				validation.ValidateObjectsRunning(ctx, t, external.clientset, external.dynamicClient, validation.K8sObjectSet{
					Namespaces: map[string][]validation.K8sObject{
						appName: {
							validation.NewK8sServiceForResource(appName, redisCacheName).
								ValidateLabels(false),
						},
					},
				})

				// The Redis service must NOT exist on the control-plane cluster.
				requireNoServicesInNamespace(ctx, t, test.Options.K8sClient, appName)
			},
		},
	})

	test.Test(t)
}
