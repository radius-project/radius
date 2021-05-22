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
		templateFilePath := filepath.Join(cwd, "../../examples/frontend-backend/template.bicep")
		applicationName := "frontend-backend"

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		// get application and verify name
		response, err := radAppClient.Get(ctx, env.ResourceGroup, applicationName, nil)
		require.NoError(t, cliutils.UnwrapErrorFromRawResponse(err))
		assert.Equal(t, applicationName, *response.ApplicationResource.Name)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand(applicationName, env.ConfigPath, DeleteTimeout)
			t.Logf("failed to delete application: %v", err)
		})

		validation.ValidatePodsRunning(t, k8s, validation.PodSet{
			Namespaces: map[string][]validation.Pod{
				applicationName: {
					validation.NewPodForComponent(applicationName, "frontend"),
					validation.NewPodForComponent(applicationName, "backend"),
				},
			},
		})
	})

	t.Run(("Deploy azure-servicebus"), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, "../../examples/azure-examples/azure-servicebus/template.bicep")

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("radius-servicebus", env.ConfigPath, DeleteTimeout)
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

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("dapr-pubsub", env.ConfigPath, DeleteTimeout)
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
		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("radius-keyvault", env.ConfigPath, DeleteTimeout)
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

		err = utils.RunRadDeployCommand(templateFilePath, env.ConfigPath, DeployTimeout)
		require.NoError(t, err)

		t.Cleanup(func() {
			err := utils.RunRadApplicationDeleteCommand("dapr-hello", env.ConfigPath, DeleteTimeout)
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
