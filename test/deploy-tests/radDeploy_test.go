// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploytests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Azure/radius/test/config"
	"github.com/Azure/radius/test/environment"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/require"
)

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

	t.Run("Deploy frontend-backend", func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../examples/frontend-backend/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("frontend-backend", env.ConfigPath, time.Minute*5)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"frontend-backend": {
					validation.NewPodForComponent("frontend-backend", "frontend"),
					validation.NewPodForComponent("frontend-backend", "backend"),
				},
			},
		})
	})

	t.Run(("Deploy azure-servicebus"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-servicebus/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("radius-servicebus", env.ConfigPath, time.Minute*5)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"radius-servicebus": {
					validation.NewPodForComponent("radius-servicebus", "sender"),
					validation.NewPodForComponent("radius-servicebus", "receiver"),
				},
			},
		})
	})

	t.Run(("Deploy dapr pubsub"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../examples/dapr-examples/dapr-pubsub-azure/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("dapr-pubsub", env.ConfigPath, time.Minute*5)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"dapr-pubsub": {
					validation.NewPodForComponent("dapr-pubsub", "nodesubscriber"),
					validation.NewPodForComponent("dapr-pubsub", "pythonpublisher"),
				},
			},
		})
	})

	t.Run(("Deploy azure keyvault"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-keyvault/template.bicep")

		// Adding pod identity takes time hence the longer timeout
		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*15)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("radius-keyvault", env.ConfigPath, time.Minute*5)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"radius-keyvault": {
					validation.NewPodForComponent("radius-keyvault", "kvaccessor"),
				},
			},
		})
	})

	t.Run(("Deploy dapr-hello (Tutorial)"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("dapr-hello", env.ConfigPath, time.Minute*5)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"dapr-hello": {
					validation.NewPodForComponent("dapr-hello", "nodeapp"),
					validation.NewPodForComponent("dapr-hello", "pythonapp"),
				},
			},
		})
	})
}
