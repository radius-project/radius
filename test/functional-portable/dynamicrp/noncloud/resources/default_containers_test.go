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

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	k8sNamespace := "default-containers-ns"

	options := rp.NewRPTestOptions(t)

	testObj := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Create the Kubernetes namespace that the Radius.Core/environments
			// resource references as its provider namespace.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: k8sNamespace},
				}
				_, err := options.K8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "default-containers-env",
						Type: "Radius.Core/environments",
					},
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

	testObj.Options = options
	testObj.Test(t)
}
