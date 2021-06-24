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

	"github.com/Azure/azure-sdk-for-go/profiles/latest/cosmos-db/mgmt/documentdb"
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
	"github.com/Azure/radius/test/radcli"
	"github.com/Azure/radius/test/utils"
	"github.com/Azure/radius/test/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

// Tests application deployment using radius
func TestDeployment(t *testing.T) {
	ctx, cancel := utils.GetContext(t)
	defer cancel()

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
		Context:       ctx,
	}

	table := []Row{
		{
			Application: "frontend-backend",
			Description: "frontend-backend",
			Template:    "../../docs/content/components/radius-components/container/frontend-backend.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"frontend-backend": {
						validation.NewK8sObjectForComponent("frontend-backend", "frontend"),
						validation.NewK8sObjectForComponent("frontend-backend", "backend"),
					},
				},
			},
			Components: validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: "frontend-backend",
						ComponentName:   "frontend",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment": validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"Service":    validation.NewOutputResource("Service", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
					},
					{
						ApplicationName: "frontend-backend",
						ComponentName:   "backend",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment": validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"Service":    validation.NewOutputResource("Service", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
						},
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
			Template:    "../../docs/content/components/radius-components/container/inboundroute.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"inbound-route": {
						validation.NewK8sObjectForComponent("inbound-route", "frontend"),
						validation.NewK8sObjectForComponent("inbound-route", "backend"),
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
			Template:    "../../docs/content/components/azure-components/azure-servicebus/template.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"radius-servicebus": {
						validation.NewK8sObjectForComponent("radius-servicebus", "sender"),
						validation.NewK8sObjectForComponent("radius-servicebus", "receiver"),
					},
				},
			},
		},
		{
			Application: "dapr-pubsub-managed",
			Description: "dapr-pubsub (Azure + Radius-managed)",
			Template:    "../../docs/content/components/dapr-components/dapr-pubsub/dapr-pubsub-servicebus/managed.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-pubsub-managed": {
						validation.NewK8sObjectForComponent("dapr-pubsub-managed", "nodesubscriber"),
						validation.NewK8sObjectForComponent("dapr-pubsub-managed", "pythonpublisher"),
					},
				},
			},
		},
		{
			Application: "dapr-pubsub-unmanaged",
			Description: "dapr-pubsub (Azure + user-managed)",
			Template:    "../../docs/content/components/dapr-components/dapr-pubsub/dapr-pubsub-servicebus/unmanaged.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-pubsub-unmanaged": {
						validation.NewK8sObjectForComponent("dapr-pubsub-unmanaged", "nodesubscriber"),
						validation.NewK8sObjectForComponent("dapr-pubsub-unmanaged", "pythonpublisher"),
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
			Template:    "../../docs/content/components/azure-components/azure-keyvault/template.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"radius-keyvault": {
						validation.NewK8sObjectForComponent("radius-keyvault", "kvaccessor"),
					},
				},
			},
			Components: validation.ComponentSet{
				Components: []validation.Component{
					{
						ApplicationName: "radius-keyvault",
						ComponentName:   "kv",
						OutputResources: map[string]validation.OutputResourceSet{
							"KeyVault": validation.NewOutputResource("KeyVault", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureKeyVault, true),
						},
					},
					{
						ApplicationName: "radius-keyvault",
						ComponentName:   "kvaccessor",
						OutputResources: map[string]validation.OutputResourceSet{
							"Deployment":                     validation.NewOutputResource("Deployment", workloads.OutputResourceTypeKubernetes, workloads.ResourceKindKubernetes, true),
							"UserAssignedManagedIdentity-KV": validation.NewOutputResource("UserAssignedManagedIdentity-KV", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureUserAssignedManagedIdentity, true),
							"RoleAssignment-KVKeys":          validation.NewOutputResource("RoleAssignment-KVKeys", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							"RoleAssignment-KVSecretsCerts":  validation.NewOutputResource("RoleAssignment-KVSecretsCerts", workloads.OutputResourceTypeArm, workloads.ResourceKindAzureRoleAssignment, true),
							"AADPodIdentity":                 validation.NewOutputResource("AADPodIdentity", workloads.OutputResourceTypePodIdentity, workloads.ResourceKindAzurePodIdentity, true),
						},
					},
				},
			},
			PostDeployVerify: func(t *testing.T, at ApplicationTest) {
				appclient := radclient.NewApplicationClient(at.Options.ARMConnection, at.Options.Environment.SubscriptionID)

				// get application and verify name
				response, err := appclient.Get(ctx, env.ResourceGroup, "radius-keyvault", nil)
				require.NoError(t, cliutils.UnwrapErrorFromRawResponse(err))
				assert.Equal(t, "radius-keyvault", *response.ApplicationResource.Name)
			},
		},
		{
			Application: "dapr-hello",
			Description: "dapr-hello (Tutorial)",
			Template:    "../../docs/content/getting-started/tutorial/dapr-microservices/dapr-microservices.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"dapr-hello": {
						validation.NewK8sObjectForComponent("dapr-hello", "nodeapp"),
						validation.NewK8sObjectForComponent("dapr-hello", "pythonapp"),
					},
				},
			},
		},
		{
			Application: "cosmos-container-managed",
			Description: "cosmos-container (radius managed)",
			Template:    "../../docs/content/components/azure-components/azure-cosmos/cosmos-mongodb/managed.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"cosmos-container-managed": {
						validation.NewK8sObjectForComponent("cosmos-container-managed", "todoapp"),
					},
				},
			},
		},
		{
			Application: "cosmos-container-unmanaged",
			Description: "cosmos-container (user managed)",
			Template:    "../../docs/content/components/azure-components/azure-cosmos/cosmos-mongodb/unmanaged.bicep",
			Pods: validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"cosmos-container-unmanaged": {
						validation.NewK8sObjectForComponent("cosmos-container-unmanaged", "todoapp"),
					},
				},
			},
			// This test has additional 'unmanaged' resources that are deployed in the same template but not managed
			// by Radius.
			//
			// We don't need to delete these, they will be deleted as part of the resource group cleanup.
			PostDeleteVerify: func(t *testing.T, at ApplicationTest) {
				// Verify that the cosmosdb resources were not deleted
				ac := documentdb.NewDatabaseAccountsClient(at.Options.Environment.SubscriptionID)
				ac.Authorizer = at.Options.Authorizer

				// We have to use a generated name due to uniqueness requirements, so lookup based on tags
				var account *documentdb.DatabaseAccountGetResults
				list, err := ac.ListByResourceGroup(context.Background(), at.Options.Environment.ResourceGroup)
				require.NoErrorf(t, err, "failed to list database accounts")

				for _, value := range *list.Value {
					if value.Tags["radiustest"] != nil {
						temp := value
						account = &temp
						break
					}
				}

				require.NotNilf(t, account, "failed to find database account with 'radiustest' tag")

				dbc := documentdb.NewMongoDBResourcesClient(at.Options.Environment.SubscriptionID)
				dbc.Authorizer = at.Options.Authorizer

				_, err = dbc.GetMongoDBDatabase(context.Background(), at.Options.Environment.ResourceGroup, *account.Name, "mydb")
				require.NoErrorf(t, err, "failed to find mongo database")
			},
		},
	}

	// Nest parallel subtests into outer Run to have function wait for all tests
	// to finish before returning.
	// See: https://golang.org/pkg/testing/#hdr-Subtests_and_Sub_benchmarks
	t.Run("deploytests", func(t *testing.T) {
		for _, row := range table {
			test := NewApplicationTest(options, row)
			t.Run(row.Description, test.Test)
		}
	})
}

type Row struct {
	Application      string
	Description      string
	Template         string
	Pods             validation.K8sObjectSet
	Components       validation.ComponentSet
	PostDeployVerify func(*testing.T, ApplicationTest)
	PostDeleteVerify func(*testing.T, ApplicationTest)
}

type Options struct {
	Environment   *environment.TestEnvironment
	K8s           *kubernetes.Clientset
	ARMConnection *armcore.Connection
	Authorizer    autorest.Authorizer
	Context       context.Context
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

	cli := radcli.NewCLI(t, at.Options.Environment.ConfigPath)

	// Inside the integration test code we rely on the context for timeout/cancellation functionality.
	// We expect the caller to wire this out to the test timeout system, or a stricter timeout if desired.

	// Deploy the application
	t.Run(fmt.Sprintf("deploy %s", at.Row.Description), func(t *testing.T) {
		templateFilePath := filepath.Join(cwd, at.Row.Template)
		t.Logf("deploying %s from file %s", at.Row.Description, at.Row.Template)
		err := cli.Deploy(at.Options.Context, templateFilePath)
		require.NoErrorf(t, err, "failed to deploy %s", at.Row.Description)
		t.Logf("finished deploying %s from file %s", at.Row.Description, at.Row.Template)

		// ValidatePodsRunning triggers its own assertions, no need to handle errors
		t.Logf("validating creation of pods for %s", at.Row.Description)
		validation.ValidatePodsRunning(at.Options.Context, t, at.Options.K8s, at.Row.Pods)
		t.Logf("finished creation of validating pods for %s", at.Row.Description)

		// Validate that all expected output resources are created
		t.Logf("validating output resources for %s", at.Row.Description)
		validation.ValidateOutputResources(t, at.Options.ARMConnection, at.Options.Environment.SubscriptionID, at.Options.Environment.ResourceGroup, at.Row.Components)
		t.Logf("finished validating output resources for %s", at.Row.Description)

		// Custom verification is expected to use `t` to trigger its own assertions
		if at.Row.PostDeployVerify != nil {
			t.Logf("running post-deploy verification for %s", at.Row.Description)
			at.Row.PostDeployVerify(t, at)
			t.Logf("finished post-deploy verification for %s", at.Row.Description)
		}
	})

	// In the future we can add more subtests here for multi-phase tests that change what's deployed.
	t.Logf("beginning cleanup phase of %s", at.Row.Description)

	// Cleanup code here will run regardless of pass/fail of subtests
	t.Logf("deleting %s", at.Row.Description)
	err = cli.ApplicationDelete(at.Options.Context, at.Row.Application)
	require.NoErrorf(t, err, "failed to delete %s", at.Row.Description)
	t.Logf("finished deleting %s", at.Row.Description)

	t.Logf("validating deletion of pods for %s", at.Row.Description)
	for ns := range at.Row.Pods.Namespaces {
		validation.ValidateNoPodsInNamespace(at.Options.Context, t, at.Options.K8s, ns)
	}
	t.Logf("finished deletion of pods for %s", at.Row.Description)

	// Custom verification is expected to use `t` to trigger its own assertions
	if at.Row.PostDeleteVerify != nil {
		t.Logf("running post-delete verification for %s", at.Row.Description)
		at.Row.PostDeleteVerify(t, at)
		t.Logf("finished post-delete verification for %s", at.Row.Description)
	}

	t.Logf("finished cleanup phase of %s", at.Row.Description)
}
