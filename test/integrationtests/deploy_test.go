// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package integrationtests

import (
	"context"
	"fmt"
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
	"k8s.io/client-go/kubernetes"
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

	k8s, err := utils.GetKubernetesClient()
	require.NoError(t, err, "failed to create kubernetes client")

	// Build rad application client
	azcred, err := azidentity.NewDefaultAzureCredential(nil)
	require.NoErrorf(t, err, "Failed to obtain Azure credentials")
	con := armcore.NewDefaultConnection(azcred, nil)

	options := Options{
		Environment:   env,
		ARMConnection: con,
		K8s:           k8s,
	}

	table := []Row{
		{
			Application: "frontend-backend",
			Description: "frontend-backend",
			Template:    "../../examples/frontend-backend/template.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"frontend-backend": {
						validation.NewPodForComponent("frontend-backend", "frontend"),
						validation.NewPodForComponent("frontend-backend", "backend"),
					},
				},
			},
			Verify: func(t *testing.T, at ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, env.ResourceGroup, "frontend-backend", nil)
				require.NoError(t, cliutils.UnwrapErrorFromRawResponse(err))
				assert.Equal(t, "frontend-backend", *response.ApplicationResource.Name)
			},
		},
		{
			Application: "radius-servicebus",
			Description: "azure-servicebus",
			Template:    "../../examples/azure-examples/azure-servicebus/template.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"radius-servicebus": {
						validation.NewPodForComponent("radius-servicebus", "sender"),
						validation.NewPodForComponent("radius-servicebus", "receiver"),
					},
				},
			},
		},
		{
			Application: "dapr-pubsub",
			Description: "dapr-pubsub (Azure)",
			Template:    "../../examples/dapr-examples/dapr-pubsub-azure/template.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"dapr-pubsub": {
						validation.NewPodForComponent("dapr-pubsub", "nodesubscriber"),
						validation.NewPodForComponent("dapr-pubsub", "pythonpublisher"),
					},
				},
			},
		},
		{
			Application: "radius-keyvault",
			Description: "azure-keyvault",
			Template:    "../../examples/azure-examples/azure-keyvault/template.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"radius-keyvault": {
						validation.NewPodForComponent("radius-keyvault", "kvaccessor"),
					},
				},
			},
		},
		{
			Application: "dapr-hello",
			Description: "dapr-hello (Tutorial)",
			Template:    "../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"dapr-hello": {
						validation.NewPodForComponent("dapr-hello", "nodeapp"),
						validation.NewPodForComponent("dapr-hello", "pythonapp"),
					},
				},
			},
		},
	}

	for _, row := range table {
		test := NewApplicationTest(options, row)
		t.Run(row.Description, test.Test)
	}
}

type Row struct {
	Application string
	Description string
	Template    string
	Pods        validation.PodSet
	Verify      func(*testing.T, ApplicationTest)
}

type Options struct {
	Environment   *environment.TestEnvironment
	K8s           *kubernetes.Clientset
	ARMConnection *armcore.Connection
}

type ApplicationTest struct {
	Options Options
	Row     Row
}

func NewApplicationTest(options Options, row Row) ApplicationTest {
	return ApplicationTest{Options: options, Row: row}
}

func (at ApplicationTest) Test(t *testing.T) {
	// This runs each application deploy as a nested test, with the cleanup as part of the surrounding test.
	// This way we can catch deletion failures and report them as test failures.
	//
	// In the future we can extend this to multi-phase tests that do more than just deploy and delete by adding more
	// intermediate sub-tests.

	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Deploy the application
	t.Run(fmt.Sprintf("deploy %s", at.Row.Description), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, at.Row.Template)
		t.Logf("deploying %s from file %s", at.Row.Description, at.Row.Template)

		err := utils.RunRadDeployCommand(templateFilePath, at.Options.Environment.ConfigPath, DeployTimeout)
		require.NoErrorf(t, err, "failed to delete %s", at.Row.Description)

		// ValidatePodsRunning triggers its own assertions, no need to handle errors
		validation.ValidatePodsRunning(t, at.Options.K8s, at.Row.Pods)

		// Custom verification is expected to use `t` to trigger its own assertions
		if at.Row.Verify != nil {
			at.Row.Verify(t, at)
		}
	})

	// In the future we can add more subtests here for multi-phase tests that change what's deployed.

	// Cleanup code here will run regardless of pass/fail of subtests
	err = utils.RunRadApplicationDeleteCommand(at.Row.Application, at.Options.Environment.ConfigPath, DeleteTimeout)
	require.NoErrorf(t, err, "failed to delete %s", at.Row.Description)

	for ns := range at.Row.Pods.Namespaces {
		validation.ValidateNoPodsInNamespace(t, at.Options.K8s, ns)
	}
}
