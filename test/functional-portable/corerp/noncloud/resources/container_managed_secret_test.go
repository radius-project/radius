/*
Copyright 2026 The Radius Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	backendsecret "github.com/radius-project/radius/pkg/dynamicrp/backend/secret"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_Container_ManagedSecretConnection(t *testing.T) {
	const (
		template      = "testdata/corerp-resources-container-managed-secret.bicep"
		name          = "corerp-container-managed-secret"
		producerName  = "managed-redis"
		containerName = "secret-consumer"
	)

	test := rp.NewRPTest(t, name, nil)

	producerID, err := resources.Parse(fmt.Sprintf("%s/providers/Radius.Data/redisCaches/%s", test.Options.Workspace.Scope, producerName))
	require.NoError(t, err)
	managedSecretName := backendsecret.ManagedSecretName(producerID)

	test.Steps = []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(
				template,
				testutil.GetMagpieImage(),
				"recipeTag=edge",
			),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: name,
						Type: validation.CoreApplicationsResource,
					},
					{
						Name: producerName,
						Type: "radius.data/redisCaches",
						App:  name,
					},
					{
						Name: managedSecretName,
						Type: validation.SecuritySecretsResource,
						App:  name,
					},
					{
						Name: containerName,
						Type: validation.ComputeContainersResource,
						App:  name,
					},
					{
						Name: name + "-env",
						Type: validation.CoreEnvironmentsResource,
					},
					{
						Name: "managed-secret-recipe-pack",
						Type: validation.CoreRecipePacksResource,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, producerName),
						validation.NewK8sServiceForResource(name, producerName),
						validation.NewK8sSecretForResourceWithResourceName(managedSecretName),
						validation.NewK8sPodForResource(name, containerName),
					},
				},
			},
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				producer, err := test.Options.ManagementClient.GetResource(ctx, "Radius.Data/redisCaches", producerName)
				require.NoError(t, err)
				require.Equal(t, producerName+"."+name+".svc.cluster.local", producer.Properties["host"])
				require.Equal(t, "6379", fmt.Sprint(producer.Properties["port"]))

				producerSecrets, ok := producer.Properties["secrets"].(map[string]any)
				require.True(t, ok, "producer should expose managed secret metadata")
				require.Equal(t, managedSecretName, producerSecrets["name"])
				if _, leaked := producerSecrets["url"]; leaked {
					require.FailNow(t, "producer state contains the secret output")
				}

				_, err = test.Options.ManagementClient.GetResource(ctx, "Radius.Security/secrets", managedSecretName)
				require.NoError(t, err, "managed Radius.Security/secrets resource should exist")

				kubernetesSecret, err := test.Options.K8sClient.CoreV1().Secrets(name).Get(ctx, managedSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				secretValue, ok := kubernetesSecret.Data["url"]
				require.True(t, ok, "managed Kubernetes Secret should contain the recipe's url output")
				require.NotEmpty(t, secretValue)

				deployment := getContainerDeployment(ctx, t, test, name, name, containerName)
				require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
				require.Len(t, deployment.Spec.Template.Spec.InitContainers, 1)

				appContainer := deployment.Spec.Template.Spec.Containers[0]
				require.Equal(t, "app", appContainer.Name)
				require.Equal(t, producerName+"."+name+".svc.cluster.local", requireEnvValue(t, appContainer, "CONNECTION_REDIS_HOST"))
				require.Equal(t, "6379", requireEnvValue(t, appContainer, "CONNECTION_REDIS_PORT"))
				requireManagedSecretEnv(t, appContainer, "CONNECTION_REDIS_URL", managedSecretName, "url")

				initContainer := deployment.Spec.Template.Spec.InitContainers[0]
				require.Equal(t, "connectioncheck", initContainer.Name)
				require.Equal(t, producerName+"."+name+".svc.cluster.local", requireEnvValue(t, initContainer, "CONNECTION_REDIS_HOST"))
				require.Equal(t, "6379", requireEnvValue(t, initContainer, "CONNECTION_REDIS_PORT"))
				requireManagedSecretEnv(t, initContainer, "CONNECTION_REDIS_URL", managedSecretName, "url")

				podSpec, err := json.Marshal(deployment.Spec.Template.Spec)
				require.NoError(t, err)
				if bytes.Contains(podSpec, secretValue) {
					require.FailNow(t, "pod spec contains the plaintext secret")
				}

				pods, err := test.Options.K8sClient.CoreV1().Pods(name).List(ctx, metav1.ListOptions{
					LabelSelector: metav1.FormatLabelSelector(deployment.Spec.Selector),
				})
				require.NoError(t, err)
				require.Len(t, pods.Items, 1)

				appLogs, err := testutil.GetPodLogs(ctx, test.Options.K8sClient, name, pods.Items[0].Name, appContainer.Name)
				require.NoError(t, err)
				require.Contains(t, appLogs, "managed-secret-connection-ready")

				initLogs, err := testutil.GetPodLogs(ctx, test.Options.K8sClient, name, pods.Items[0].Name, initContainer.Name)
				require.NoError(t, err)
				require.Contains(t, initLogs, "managed-secret-init-ready")
			},
		},
	}

	test.Test(t)
}

func requireEnvValue(t *testing.T, container corev1.Container, name string) string {
	t.Helper()

	env := requireEnv(t, container, name)
	require.Nil(t, env.ValueFrom, "%s should be an ordinary environment value", name)
	require.NotEmpty(t, env.Value, "%s should not be empty", name)
	return env.Value
}

func requireManagedSecretEnv(t *testing.T, container corev1.Container, name, secretName, secretKey string) {
	t.Helper()

	env := requireEnv(t, container, name)
	if env.Value != "" {
		require.FailNow(t, "secret environment variable contains plaintext", "%s must use valueFrom", name)
	}
	require.NotNil(t, env.ValueFrom, "%s should be sourced from a secret", name)
	require.NotNil(t, env.ValueFrom.SecretKeyRef, "%s should use secretKeyRef", name)
	require.Equal(t, secretName, env.ValueFrom.SecretKeyRef.Name)
	require.Equal(t, secretKey, env.ValueFrom.SecretKeyRef.Key)
}

func requireEnv(t *testing.T, container corev1.Container, name string) corev1.EnvVar {
	t.Helper()

	for _, env := range container.Env {
		if env.Name == name {
			return env
		}
	}

	require.FailNow(t, "environment variable not found", "container %q does not contain %q", container.Name, name)
	return corev1.EnvVar{}
}
