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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
)

func Test_Environment(t *testing.T) {
	template := "testdata/corerp-resources-environment.bicep"
	name := "corerp-resources-environment"

	test := rp.NewRPTest(t, name, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "corerp-resources-environment-env",
						Type: validation.CoreEnvironmentsResource,
					},
				},
			},
			// Environment should not render any K8s Objects directly
			K8sObjects: &validation.K8sObjectSet{},
		},
	})

	test.Test(t)
}

// Test_Environment_CascadeDelete verifies that deleting a Radius.Core environment also deletes the
// applications in that environment and the resources deployed into it, matching the behavior of
// Applications.Core environments.
//
// Framework-driven teardown is disabled so only the environment cascade removes the application,
// container, RabbitMQ, its generated secret, and a standalone secret.
func Test_Environment_CascadeDelete(t *testing.T) {
	template := "testdata/corerp-resources-env-cascade.bicep"
	name := "corerp-resources-env-cascade"
	containerName := "env-cascade-ctnr"
	rabbitmqName := "env-cascade-rabbitmq"
	standaloneSecretName := "env-cascade-standalone"

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
					{Name: rabbitmqName, Type: validation.MessagingRabbitMQResource, App: name},
					{Name: standaloneSecretName, Type: validation.SecuritySecretsResource, App: name},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					name: {
						validation.NewK8sPodForResource(name, containerName),
					},
				},
			},
			// The cascade triggered by PostStepVerify is what deletes these resources.
			SkipResourceDeletion: true,
		},
	})

	preSetup, previewEnvID := rp.NewPreviewEnvPreSetup(name, test.Options.Workspace.Scope, name)
	envName := name + "-env"

	test.PreSetup = preSetup
	test.Steps[0].Executor = step.NewDeployExecutor(template, testutil.GetMagpieImage(), fmt.Sprintf("environment=%s", previewEnvID))
	test.Steps[0].PostStepVerify = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		scope := ct.Options.Workspace.Scope
		applicationID := fmt.Sprintf("%s/providers/Radius.Core/applications/%s", scope, name)
		containerID := fmt.Sprintf("%s/providers/Radius.Compute/containers/%s", scope, containerName)
		rabbitmqID := fmt.Sprintf("%s/providers/Radius.Messaging/rabbitMQ/%s", scope, rabbitmqName)
		managedSecretName := requireManagedSecret(t, ctx, ct, validation.MessagingRabbitMQResource, rabbitmqID, name)

		cli := radcli.NewCLI(t, ct.Options.ConfigFilePath)

		t.Logf("deleting environment %s, expecting the delete to cascade", envName)
		_, err := cli.EnvironmentDeletePreview(ctx, envName, "")
		require.NoError(t, err, "failed to delete preview environment")

		requireManagedSecretDeleted(t, ctx, ct, name, managedSecretName)
		requireManagedSecretDeleted(t, ctx, ct, name, standaloneSecretName)
		requireResourceDeleted(ctx, t, ct, validation.MessagingRabbitMQResource, rabbitmqID)
		requireResourceDeleted(ctx, t, ct, validation.ComputeContainersResource, containerID)
		requireResourceDeleted(ctx, t, ct, validation.CoreApplicationsResource, applicationID)
		requireResourceDeleted(ctx, t, ct, validation.CoreEnvironmentsResource, previewEnvID)

		validation.ValidateNoPodsInApplication(ctx, t, ct.Options.K8sClient, name, name)

		// Deleting an environment that no longer exists is a no-op rather than an error, so
		// teardown and repeated invocations stay safe.
		_, err = cli.EnvironmentDeletePreview(ctx, envName, "")
		require.NoError(t, err, "deleting an already-deleted environment should succeed")
	}

	test.Test(t)
}

func requireManagedSecret(t *testing.T, ctx context.Context, ct rp.RPTest, resourceType, resourceID, namespace string) string {
	t.Helper()
	resource, err := ct.Options.ManagementClient.GetResource(ctx, resourceType, resourceID)
	require.NoError(t, err)
	secrets, ok := resource.Properties["secrets"].(map[string]any)
	require.True(t, ok, "resource should expose managed secret metadata")
	name, ok := secrets["name"].(string)
	require.True(t, ok, "managed secret name should be a string")
	require.NotEmpty(t, name)
	_, err = ct.Options.ManagementClient.GetResource(ctx, validation.SecuritySecretsResource, name)
	require.NoError(t, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := ct.Options.K8sClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		assert.NoError(c, err, "backing Kubernetes secret should exist before deletion")
	}, time.Minute, time.Second)
	return name
}

func requireManagedSecretDeleted(t *testing.T, ctx context.Context, ct rp.RPTest, namespace, name string) {
	t.Helper()
	_, err := ct.Options.ManagementClient.GetResource(ctx, validation.SecuritySecretsResource, name)
	require.True(t, clients.Is404Error(err), "CLI must not finish before Radius secret %s is deleted: %v", name, err)
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		_, err := ct.Options.K8sClient.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		assert.True(c, apierrors.IsNotFound(err), "backing Kubernetes secret should be deleted: %v", err)
	}, 30*time.Second, time.Second)
}

// requireResourceDeleted asserts that the given resource is reported as not found. Deletes are
// awaited by the CLI, but the assertion is retried to absorb any propagation delay.
func requireResourceDeleted(ctx context.Context, t *testing.T, ct rp.RPTest, resourceType string, resourceID string) {
	t.Helper()

	var lastErr error
	deadline := time.Now().Add(60 * time.Second)
	for {
		_, lastErr = ct.Options.ManagementClient.GetResource(ctx, resourceType, resourceID)
		if lastErr != nil && clients.Is404Error(lastErr) {
			t.Logf("verified %s was deleted by the environment cascade", resourceID)
			return
		}

		// Stop retrying once the test context is done, so a canceled or timed-out run fails
		// immediately instead of burning the full deadline.
		if err := ctx.Err(); err != nil {
			require.Failf(t, "context ended before the resource was deleted",
				"gave up waiting for %s to be deleted: %v (last error: %v)", resourceID, err, lastErr)
			return
		}

		if time.Now().After(deadline) {
			break
		}

		time.Sleep(2 * time.Second)
	}

	require.Failf(t, "resource was not deleted",
		"expected %s to be deleted by the environment cascade, but it is still present (last error: %v)", resourceID, lastErr)
}
