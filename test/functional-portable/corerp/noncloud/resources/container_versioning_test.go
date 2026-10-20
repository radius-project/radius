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

	"github.com/radius-project/radius/pkg/kubernetes"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// Test_ContainerVersioning verifies that redeploying a container with an updated definition
// versions the container's runtime configuration. Version 1 consumes a Radius.Security/secrets
// resource via a secretKeyRef environment variable; version 2 removes that environment variable.
// The test asserts the secret-backed env var is present after v1 and absent after v2.
func Test_ContainerVersioning(t *testing.T) {
	containerV1 := "testdata/containers/corerp-resources-friendly-container-version-1.bicep"
	containerV2 := "testdata/containers/corerp-resources-friendly-container-version-2.bicep"

	name := "corerp-resources-container-versioning"
	appNamespace := name
	containerName := "friendly-ctnr"
	secretName := "friendly-secret"

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
					{
						Name: secretName,
						Type: validation.SecuritySecretsResource,
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
			SkipResourceDeletion: true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deployment := getContainerDeployment(ctx, t, test, appNamespace, name, containerName)

				var found bool
				for _, envVar := range deployment.Spec.Template.Spec.Containers[0].Env {
					if envVar.Name == "DB_PASSWORD" {
						found = true
						require.NotNil(t, envVar.ValueFrom, "expected DB_PASSWORD to be sourced from a secret")
						require.NotNil(t, envVar.ValueFrom.SecretKeyRef, "expected DB_PASSWORD to use a secretKeyRef")
						require.Equal(t, secretName, envVar.ValueFrom.SecretKeyRef.Name, "expected DB_PASSWORD to reference the friendly-secret")
						break
					}
				}
				require.True(t, found, "expected secret-backed env var DB_PASSWORD on version 1 of the container")
			},
		},
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
					{
						Name: secretName,
						Type: validation.SecuritySecretsResource,
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
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				deployment := getContainerDeployment(ctx, t, test, appNamespace, name, containerName)

				for _, envVar := range deployment.Spec.Template.Spec.Containers[0].Env {
					require.NotEqual(t, "DB_PASSWORD", envVar.Name, "expected secret-backed env var DB_PASSWORD to be removed on version 2 of the container")
				}
			},
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, appNamespace)
	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(containerV1, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))
	test.Steps[1].Executor = step.NewDeployExecutor(containerV2, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))

	test.Test(t)
}

// getContainerDeployment returns the single Kubernetes Deployment for the given Radius container
// resource, failing the test if it is not found.
func getContainerDeployment(ctx context.Context, t *testing.T, test rp.RPTest, namespace, appName, resourceName string) appsv1.Deployment {
	labelset := kubernetes.MakeSelectorLabels(appName, resourceName)

	deployments, err := test.Options.K8sClient.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(labelset).String(),
	})
	require.NoError(t, err, "failed to list deployments")
	require.Len(t, deployments.Items, 1, "expected exactly 1 deployment for container %s", resourceName)

	return deployments.Items[0]
}
