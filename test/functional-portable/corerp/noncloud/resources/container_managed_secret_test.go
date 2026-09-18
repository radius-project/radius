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
	"net/http"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	backendsecret "github.com/radius-project/radius/pkg/dynamicrp/backend/secret"
	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	// The validation set needs the deterministic name before deployment; the assertion against
	// producer.Properties["secrets"]["name"] below verifies the public contract independently.
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
						Type: validation.DataRedisCachesResource,
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
				producer, err := test.Options.ManagementClient.GetResource(ctx, validation.DataRedisCachesResource, producerName)
				require.NoError(t, err)
				require.Equal(t, producerName+"."+name+".svc.cluster.local", producer.Properties["host"])
				require.EqualValues(t, 6379, producer.Properties["port"])

				producerSecrets, ok := producer.Properties["secrets"].(map[string]any)
				require.True(t, ok, "producer should expose managed secret metadata")
				require.Equal(t, managedSecretName, producerSecrets["name"])
				if _, leaked := producerSecrets["url"]; leaked {
					require.FailNow(t, "producer state contains the secret output")
				}

				managedSecret, err := test.Options.ManagementClient.GetResource(ctx, validation.SecuritySecretsResource, managedSecretName)
				require.NoError(t, err, "managed Radius.Security/secrets resource should exist")
				if _, leaked := managedSecret.Properties["url"]; leaked {
					require.FailNow(t, "managed secret state contains the secret output")
				}

				kubernetesSecret, err := test.Options.K8sClient.CoreV1().Secrets(name).Get(ctx, managedSecretName, metav1.GetOptions{})
				require.NoError(t, err)
				secretValue, ok := kubernetesSecret.Data["url"]
				require.True(t, ok, "managed Kubernetes Secret should contain the recipe's url output")
				require.NotEmpty(t, secretValue)

				managedSecretProperties, err := json.Marshal(managedSecret.Properties)
				require.NoError(t, err)
				if bytes.Contains(managedSecretProperties, secretValue) {
					require.FailNow(t, "managed secret state contains the plaintext secret")
				}

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

				deploymentState, err := json.Marshal(deployment)
				require.NoError(t, err)
				if bytes.Contains(deploymentState, secretValue) {
					require.FailNow(t, "deployment contains the plaintext secret")
				}

				pod := requireReadyPod(ctx, t, test, name, deployment)
				require.EventuallyWithT(t, func(collect *assert.CollectT) {
					appLogs, err := testutil.GetPodLogs(ctx, test.Options.K8sClient, name, pod.Name, appContainer.Name)
					assert.NoError(collect, err)
					assert.Contains(collect, appLogs, "managed-secret-connection-ready")

					initLogs, err := testutil.GetPodLogs(ctx, test.Options.K8sClient, name, pod.Name, initContainer.Name)
					assert.NoError(collect, err)
					assert.Contains(collect, initLogs, "managed-secret-init-ready")
				}, 30*time.Second, time.Second)
			},
		},
	}

	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, test rp.RPTest) {
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			_, err := test.Options.ManagementClient.GetResource(ctx, validation.SecuritySecretsResource, managedSecretName)
			var responseError *azcore.ResponseError
			if assert.ErrorAs(collect, err, &responseError, "managed Radius.Security/secrets resource should be deleted") {
				assert.Equal(collect, http.StatusNotFound, responseError.StatusCode)
			}

			_, err = test.Options.K8sClient.CoreV1().Secrets(name).Get(ctx, managedSecretName, metav1.GetOptions{})
			assert.True(collect, apierrors.IsNotFound(err), "managed Kubernetes Secret should be deleted, got: %v", err)
		}, 30*time.Second, time.Second)
	}

	test.Test(t)
}

func requireReadyPod(ctx context.Context, t *testing.T, test rp.RPTest, namespace string, deployment appsv1.Deployment) corev1.Pod {
	t.Helper()

	deploymentSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	deploymentRevision := deployment.Annotations["deployment.kubernetes.io/revision"]
	require.NotEmpty(t, deploymentRevision, "deployment should have a revision")

	replicaSets, err := test.Options.K8sClient.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: deploymentSelector,
	})
	require.NoError(t, err)
	var currentReplicaSet *appsv1.ReplicaSet
	for i := range replicaSets.Items {
		replicaSet := &replicaSets.Items[i]
		if metav1.IsControlledBy(replicaSet, &deployment) &&
			replicaSet.Annotations["deployment.kubernetes.io/revision"] == deploymentRevision {
			currentReplicaSet = replicaSet
			break
		}
	}
	require.NotNil(t, currentReplicaSet, "current ReplicaSet not found for deployment %s", deployment.Name)

	podSelector := metav1.FormatLabelSelector(currentReplicaSet.Spec.Selector)
	pods, err := test.Options.K8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: podSelector,
	})
	require.NoError(t, err)

	for _, pod := range pods.Items {
		if !metav1.IsControlledBy(&pod, currentReplicaSet) ||
			pod.Status.Phase != corev1.PodRunning ||
			pod.DeletionTimestamp != nil {
			continue
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
				return pod
			}
		}
	}

	require.FailNowf(t, "container pod not ready", "no ready pods found for current ReplicaSet %s in namespace %s", currentReplicaSet.Name, namespace)
	return corev1.Pod{}
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
