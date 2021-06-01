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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/servicebus/mgmt/servicebus"
	"github.com/Azure/azure-sdk-for-go/sdk/armcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/go-autorest/autorest"
	cliutils "github.com/Azure/radius/cmd/cli/utils"
	"github.com/Azure/radius/pkg/rad/azure"
	"github.com/Azure/radius/pkg/rad/bicep"
	"github.com/Azure/radius/pkg/radclient"
	"github.com/Azure/radius/pkg/workloads"
	"github.com/Azure/radius/test/config"
	"github.com/Azure/radius/test/environment"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	require.NoErrorf(t, err, "failed to obtain Azure credentials")
	con := armcore.NewDefaultConnection(azcred, nil)

	// Access Azure credentials
	armauth, err := azure.GetResourceManagerEndpointAuthorizer()
	require.NoErrorf(t, err, "failed to obtain Azure credentials")

	// Ensure rad-bicep has been downloaded before we go parallel
	installed, err := bicep.IsBicepInstalled()
	require.NoErrorf(t, err, "failed to local rad-bicep")
	if !installed {
		err = bicep.DownloadBicep()
		require.NoErrorf(t, err, "failed to download rad-bicep")
	}

	options := Options{
		Environment:   env,
		ARMConnection: con,
		K8s:           k8s,
		Authorizer:    armauth,
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
			PostDeployVerify: func(t *testing.T, at ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, env.ResourceGroup, "frontend-backend", nil)
				require.NoError(t, cliutils.UnwrapErrorFromRawResponse(err))
				assert.Equal(t, "frontend-backend", *response.ApplicationResource.Name)
			},
		},
		{
			Application: "inbound-route",
			Description: "inbound-route",
			Template:    "../../examples/inbound-route/template.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"inbound-route": {
						validation.NewPodForComponent("inbound-route", "frontend"),
						validation.NewPodForComponent("inbound-route", "backend"),
					},
				},
			},
			PostDeployVerify: func(t *testing.T, at ApplicationTest) {
				// Verify that we've created an ingress resource. We don't verify reachability because allocating
				// a public IP can take a few minutes.
				labelset := map[string]string{
					workloads.LabelRadiusApplication: "inbound-route",
					workloads.LabelRadiusComponent:   "frontend",
				}
				matches, err := at.Options.K8s.NetworkingV1().Ingresses("inbound-route").List(context.Background(), v1.ListOptions{
					LabelSelector: labels.SelectorFromSet(labelset).String(),
				})
				require.NoError(t, err, "failed to list ingresses")
				require.Lenf(t, matches.Items, 1, "items should contain one match, instead it had: %+v", matches.Items)
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
			Application: "dapr-pubsub-managed",
			Description: "dapr-pubsub (Azure + Radius-managed)",
			Template:    "../../examples/dapr-examples/dapr-pubsub-azure/managed.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"dapr-pubsub-managed": {
						validation.NewPodForComponent("dapr-pubsub-managed", "nodesubscriber"),
						validation.NewPodForComponent("dapr-pubsub-managed", "pythonpublisher"),
					},
				},
			},
		},
		{
			Application: "dapr-pubsub-unmanaged",
			Description: "dapr-pubsub (Azure + user-managed)",
			Template:    "../../examples/dapr-examples/dapr-pubsub-azure/unmanaged.bicep",
			Pods: validation.PodSet{
				Namespaces: map[string][]validation.Pod{
					"dapr-pubsub-unmanaged": {
						validation.NewPodForComponent("dapr-pubsub-unmanaged", "nodesubscriber"),
						validation.NewPodForComponent("dapr-pubsub-unmanaged", "pythonpublisher"),
					},
				},
			},
			// This test has additional 'unmanaged' resources that are deployed in the same template but not managed
			// by Radius.
			//
			// We don't need to delete these, they will be deleted as part of the resource group cleanup.
			PostDeleteVerify: func(t *testing.T, at ApplicationTest) {
				// Verify that the servicebus resources were not deleted
				nsc := servicebus.NewNamespacesClient(at.Options.Environment.SubscriptionID)
				nsc.Authorizer = at.Options.Authorizer

				// We have to use a generated name due to uniqueness requirements, so lookup based on tags
				var ns *servicebus.SBNamespace
				list, err := nsc.ListByResourceGroup(context.Background(), at.Options.Environment.ResourceGroup)
				require.NoErrorf(t, err, "failed to list servicebus namespaces")

			outer:
				for ; list.NotDone(); err = list.Next() {
					require.NoErrorf(t, err, "failed to list servicebus namespaces")

					for _, value := range list.Values() {
						if value.Tags["radiustest"] != nil {
							temp := value
							ns = &temp
							break outer
						}
					}
				}

				require.NotNilf(t, ns, "failed to find servicebus namespace with 'radiustest' tag")

				tc := servicebus.NewTopicsClient(at.Options.Environment.SubscriptionID)
				tc.Authorizer = at.Options.Authorizer

				_, err = tc.Get(context.Background(), at.Options.Environment.ResourceGroup, *ns.Name, "TOPIC_A")
				require.NoErrorf(t, err, "failed to find servicebus topic")
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
	Application      string
	Description      string
	Template         string
	Pods             validation.PodSet
	PostDeployVerify func(*testing.T, ApplicationTest)
	PostDeleteVerify func(*testing.T, ApplicationTest)
}

type Options struct {
	Environment   *environment.TestEnvironment
	K8s           *kubernetes.Clientset
	ARMConnection *armcore.Connection
	Authorizer    autorest.Authorizer
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

	// Each of our tests are isolated to a single application, so they can run in parallel.
	t.Parallel()

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
		if at.Row.PostDeployVerify != nil {
			at.Row.PostDeployVerify(t, at)
		}
	})

	// In the future we can add more subtests here for multi-phase tests that change what's deployed.

	// Cleanup code here will run regardless of pass/fail of subtests
	err = utils.RunRadApplicationDeleteCommand(at.Row.Application, at.Options.Environment.ConfigPath, DeleteTimeout)
	require.NoErrorf(t, err, "failed to delete %s", at.Row.Description)

	for ns := range at.Row.Pods.Namespaces {
		validation.ValidateNoPodsInNamespace(t, at.Options.K8s, ns)
	}

	// Custom verification is expected to use `t` to trigger its own assertions
	if at.Row.PostDeleteVerify != nil {
		at.Row.PostDeleteVerify(t, at)
	}
}
