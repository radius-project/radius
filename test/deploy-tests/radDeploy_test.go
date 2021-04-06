// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package deploytests

import (
	"context"
	"fmt"
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

	// Schedule test cluster cleanup
	defer cleanup(ctx, t, config, *env)

	err = env.DeployRP(ctx, config.Authorizer)
	require.NoError(t, err)

	// Merge the k8s credentials to the cluster if it's a leased one
	if env.UsingReservedTestCluster {
		err = utils.RunRadMergeCredentialsCommand(env.ConfigPath)
		require.NoError(t, err)
	}

	cwd, err := os.Getwd()
	require.NoError(t, err)

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	t.Run("Deploy frontend-backend", func(t *testing.T) {
		appName := "frontend-backend"
		templateFilePath := filepath.Join(cwd, "../../examples/", appName, "/azure-bicep/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

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
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-servicebus/azure-bicep/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

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
		templateFilePath := filepath.Join(cwd, "../../examples/dapr-examples/dapr-pubsub-azure/azure-bicep/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				"dapr-pubsub": {
					validation.NewPodForComponent("dapr-pubsub", "nodesubscriber"),
					validation.NewPodForComponent("dapr-pubsub", "pythonpublisher"),
				},
			},
		})
	})

	t.Run(("Deploy dapr-hello (Tutorial)"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../docs/content/tutorial/dapr-microservices/dapr-microservices.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, time.Minute*5)
		require.NoError(t, err)

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

func cleanup(ctx context.Context, t *testing.T, config *config.AzureConfig, env environment.TestEnvironment) {
	// Delete the template deployment
	err := utils.RunRadDeleteApplicationsCommand(env.ResourceGroup)
	if err != nil {
		t.Log(err.Error())
	}

	// Nothing we can really do here other than log it. Using PrintLn because we want to log it unconditionally
	err = environment.ReleaseTestEnvironment(ctx, config, env)
	if err != nil {
		fmt.Printf("failed to release test environment: %v", err)
	}
}
