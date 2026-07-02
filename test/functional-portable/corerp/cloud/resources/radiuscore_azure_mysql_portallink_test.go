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
	"os"
	"strings"
	"testing"
	"time"

	aztoken "github.com/radius-project/radius/pkg/azure/tokencredentials"
	"github.com/radius-project/radius/pkg/cli/clients"
	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_RadiusCore_AzureMySql_PortalLink verifies that the application graph
// surfaces an Azure portal deep link (portalUrl) on the output resource(s)
// produced when a Radius.Data/mySqlDatabases instance is provisioned via a
// Terraform recipe that creates real Azure resources.
//
// Flow:
//  1. Preflight: confirm the built-in Radius.Data/mySqlDatabases resource
//     type is registered (default-registered per deploy/manifest/defaults.yaml).
//  2. Deploy: a Radius.Core/recipePacks + Radius.Core/environments (with an
//     Azure provider) + Radius.Core/applications + Radius.Security/secrets +
//     Radius.Data/mySqlDatabases (whose Terraform recipe provisions an Azure
//     MySQL Flexible Server).
//  3. Verify: call GetGraph via the v20250801preview client and assert that
//     one of the mysql resource's output resources is an Azure MySQL
//     Flexible Server carrying a well-formed portalUrl.
func Test_RadiusCore_AzureMySql_PortalLink(t *testing.T) {
	template := "testdata/corerp-radiuscore-azure-mysql-portallink.bicep"
	testName := "corerp-radiuscore-azure-mysql-portallink"

	uniqueSeed := os.Getenv("UNIQUE_ID")
	if uniqueSeed == "" {
		uniqueSeed = "local"
	}
	// Bicep resource names must be short; uniqueSeed is already <=10 chars
	// (UNIQUE_ID format) but truncate defensively.
	if len(uniqueSeed) > 10 {
		uniqueSeed = uniqueSeed[:10]
	}
	uniqueSeed = strings.ToLower(uniqueSeed)

	appName := "azure-mysql-portallink-app"
	envName := "azure-mysql-portallink-env-" + uniqueSeed
	secretName := "azure-mysql-portallink-secret-" + uniqueSeed
	mysqlName := "azure-mysql-portallink-db-" + uniqueSeed
	recipePackName := "azure-mysql-portallink-pack"
	appNamespace := "azure-mysql-portallink-ns"

	azureSubscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")
	azureResourceGroupName := os.Getenv("INTEGRATION_TEST_RESOURCE_GROUP_NAME")

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	testSteps := []rp.TestStep{
		{
			// Radius.Core/environments requires the target Kubernetes namespace to
			// exist beforehand (unlike Applications.Core/environments, which
			// auto-creates it). Pre-create it so the environment resource can bind.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, opts test.TestOptions) {
				_, err := opts.K8sClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: appNamespace},
				}, metav1.CreateOptions{})
				if err != nil && !strings.Contains(err.Error(), "already exists") {
					require.NoError(t, err)
				}
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			// Preflight: confirm Radius.Data/mySqlDatabases is registered.
			// This built-in resource type ships as a default (see
			// deploy/manifest/defaults.yaml) and requires no CLI registration.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, _ test.TestOptions) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", "Radius.Data/mySqlDatabases", "--output", "json"})
				require.NoError(t, err, "rad resource-type show should succeed for the default-registered type")
				require.Contains(t, output, "mySqlDatabases")
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			Executor: step.NewDeployExecutor(
				template,
				testutil.GetTerraformRecipeModuleServerURL(),
				"appName="+appName,
				"uniqueSeed="+uniqueSeed,
				"azureSubscriptionId="+azureSubscriptionID,
				"azureResourceGroupName="+azureResourceGroupName,
				"password=not-prod-password",
			).WithRetry(2, 60*time.Second, isTransientAzureError),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{Name: recipePackName, Type: validation.CoreRecipePacksResource},
					{Name: envName, Type: validation.CoreEnvironmentsResource},
					{Name: appName, Type: validation.CoreApplicationsResource, App: appName},
					{Name: secretName, Type: validation.SecuritySecretsResource},
					{Name: mysqlName, Type: validation.DataMySQLDatabasesResource, App: appName},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				client, ok := ct.Options.ManagementClient.(*clients.UCPApplicationsManagementClient)
				require.True(t, ok, "expected UCPApplicationsManagementClient")

				appGraphClient, err := v20250801preview.NewApplicationsClient(&aztoken.AnonymousCredential{}, client.ClientOptions)
				require.NoError(t, err)

				res, err := appGraphClient.GetGraph(ctx, client.RootScope, appName, v20250801preview.GetGraphRequest{}, nil)
				require.NoError(t, err)
				require.NotNil(t, res.Resources)

				var mysqlPortalURL *string
				var mysqlOutputResourceID string
				for _, r := range res.Resources {
					if r == nil || r.Type == nil {
						continue
					}
					if !strings.EqualFold(*r.Type, "Radius.Data/mySqlDatabases") {
						continue
					}
					for _, or := range r.OutputResources {
						if or == nil || or.ID == nil {
							continue
						}
						// Look for the Azure MySQL Flexible Server output resource.
						if strings.Contains(strings.ToLower(*or.ID), "/microsoft.dbformysql/flexibleservers/") {
							mysqlOutputResourceID = *or.ID
							mysqlPortalURL = or.PortalURL
							break
						}
					}
				}

				require.NotEmpty(t, mysqlOutputResourceID,
					"expected the Radius.Data/mySqlDatabases resource in the graph to surface an Azure MySQL Flexible Server output resource")
				require.NotNil(t, mysqlPortalURL,
					"expected the Azure MySQL Flexible Server output resource to carry a portalUrl")
				require.True(t, strings.HasPrefix(*mysqlPortalURL, "https://portal.azure.com/#@"),
					"portalUrl should be an Azure portal deep link, got %q", *mysqlPortalURL)
				expectedSuffix := "/resource" + mysqlOutputResourceID
				require.True(t, strings.HasSuffix(*mysqlPortalURL, expectedSuffix),
					"portalUrl should reference the MySQL flexible server ARM ID, got %q (expected suffix %q)", *mysqlPortalURL, expectedSuffix)
			},
		},
	}

	test := rp.NewRPTest(t, testName, testSteps)
	test.RequiredFeatures = []rp.RequiredFeature{rp.FeatureAzure}
	test.RunSerial = true

	// Delete the pre-created Kubernetes namespace after RP cleanup finishes.
	// PostDeleteVerify runs after Radius has deleted the mysql resource (which
	// triggers `terraform destroy` of the Azure MySQL flexible server), so the
	// namespace is not torn down while an in-flight destroy still needs it.
	test.PostDeleteVerify = func(ctx context.Context, t *testing.T, ct rp.RPTest) {
		err := ct.Options.K8sClient.CoreV1().Namespaces().Delete(ctx, appNamespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Warning: Failed to delete namespace %s: %v", appNamespace, err)
		}
	}

	// Skip if the CI env vars needed by the bicep are missing (e.g. local runs
	// without Azure credentials). The bicep template requires a real Azure
	// subscription and resource group to provision the MySQL flexible server.
	if azureSubscriptionID == "" {
		t.Skip("AZURE_SUBSCRIPTION_ID is not set; skipping Azure MySQL portal link test")
	}
	if azureResourceGroupName == "" {
		t.Skip("INTEGRATION_TEST_RESOURCE_GROUP_NAME is not set; skipping Azure MySQL portal link test")
	}

	test.Test(t)
}
