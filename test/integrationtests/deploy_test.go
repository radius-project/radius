// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package integrationtests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	cliutils "github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/test/config"
	"github.com/Azure/radius/test/environment"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const DeployTimeout = 30 * time.Minute
const DeleteTimeout = 30 * time.Minute

// Tests application deployment using radius
func TestDeployment(t *testing.T) {
	ctx := context.Background()

	config, err := config.NewAzureConfig()
	require.NoError(t, err, "failed to initialize azure config")

	// Find a test cluster
	env, err := environment.GetTestEnvironment(ctx, config)
	require.NoError(t, err)

	cwd, err := os.Getwd()
	require.NoError(t, err)

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	// Build rad application client
	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoErrorf(t, err, "Failed to obtain Azure credentials")
	con := armcore.NewDefaultConnection(azcred, nil)
	radAppClient := radclient.NewApplicationClient(con, env.SubscriptionID)

	t.Run("Deploy frontend-backend", func(t *testing.T) {
		applicationName := "frontend-backend"
		templateFilePath := filepath.Join(cwd, "../../examples/frontend-backend/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		// get application and verify name
		response, err := radAppClient.Get(ctx, env.ResourceGroup, applicationName, nil)
		require.NoError(t, cliutils.UnwrapErrorFromRawResponse(err))
		assert.Equal(t, applicationName, *response.ApplicationResource.Name)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "frontend"),
					validation.NewPodForComponent(applicationName, "backend"),
				},
			},
		})

		t.Cleanup(func() {
			if err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, time.Minute*5); err != nil {
				t.Errorf("failed to delete application: %w", err)
			}
			if ok := validation.ValidateNoPodsInNamespace(t, k8s, applicationName); !ok {
				t.Logf("Some pods in the namespace %v are still not delete", applicationName)
			}
		})
	})

	t.Run(("Deploy azure-servicebus"), func(t *testing.T) {
		applicationName := "radius-servicebus"
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-servicebus/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "sender"),
					validation.NewPodForComponent(applicationName, "receiver"),
				},
			},
		})

		t.Cleanup(func() {
			if err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, time.Minute*5); err != nil {
				t.Errorf("failed to delete application: %w", err)
			}

			if ok := validation.ValidateNoPodsInNamespace(t, k8s, applicationName); !ok {
				t.Logf("Some pods in the namespace %v are still not delete", applicationName)
			}
		})
	})

	t.Run(("Deploy dapr pubsub"), func(t *testing.T) {
		applicationName := "dapr-pubsub"
		templateFilePath := filepath.Join(cwd, "../../examples/dapr-examples/dapr-pubsub-azure/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "nodesubscriber"),
					validation.NewPodForComponent(applicationName, "pythonpublisher"),
				},
			},
		})

		t.Cleanup(func() {
			if err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, time.Minute*5); err != nil {
				t.Errorf("failed to delete application: %w", err)
			}

			if ok := validation.ValidateNoPodsInNamespace(t, k8s, applicationName); !ok {
				t.Logf("Some pods in the namespace %v are still not delete", applicationName)
			}
		})
	})

	t.Run(("Deploy azure keyvault"), func(t *testing.T) {
		applicationName := "radius-keyvault"
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-keyvault/template.bicep")

		// Adding pod identity takes time hence the longer timeout
		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "kvaccessor"),
				},
			},
		})

		t.Cleanup(func() {
			if err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, time.Minute*5); err != nil {
				t.Errorf("failed to delete application: %w", err)
			}

			if ok := validation.ValidateNoPodsInNamespace(t, k8s, applicationName); !ok {
				t.Logf("Some pods in the namespace %v are still not delete", applicationName)
			}
		})
	})

	t.Run(("Deploy dapr-hello (Tutorial)"), func(t *testing.T) {
		applicationName := "dapr-hello"
		templateFilePath := filepath.Join(cwd, "../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "nodeapp"),
					validation.NewPodForComponent(applicationName, "pythonapp"),
				},
			},
		})

		t.Cleanup(func() {
			if err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, time.Minute*5); err != nil {
				t.Errorf("failed to delete application: %w", err)
			}

			if ok := validation.ValidateNoPodsInNamespace(t, k8s, applicationName); !ok {
				t.Logf("Some pods in the namespace %v are still not delete", applicationName)
			}
		})
	})
}
