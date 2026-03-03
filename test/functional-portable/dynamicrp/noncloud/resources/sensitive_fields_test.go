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
	"testing"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_DynamicRP_SensitiveFieldEncryption tests that fields marked with x-radius-sensitive annotation
// in a resource type schema are encrypted on PUT, redacted (nullified) by the backend after recipe
// execution, and returned as null on GET/LIST.
//
// The test consists of three main steps:
//
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/sensitiveResource" (recipe-capable)
//   - The schema includes sensitive fields (password, apiKey, credentials.secret), a sensitive object (connectionConfig), and non-sensitive fields (username, credentials.host)
//
// 2. Resource Deployment (Create):
//   - Deploys a Bicep template that creates a sensitiveResource instance with plaintext sensitive values
//   - A recipe creates a K8s Secret containing the decrypted sensitive values, proving decryption worked
//   - Verifies via GET that non-sensitive fields are returned as plaintext
//   - Verifies via GET that sensitive fields are null (redacted by backend)
//   - Verifies via LIST that the same redaction behavior applies
//   - Verifies the K8s Secret contains the expected plaintext values
//
// 3. Resource Update:
//   - Deploys an updated Bicep template with different sensitive values
//   - Verifies that sensitive fields are null after redaction
//   - Verifies the K8s Secret contains the updated plaintext values
func Test_DynamicRP_SensitiveFieldEncryption(t *testing.T) {
	template := "testdata/sensitive-resource.bicep"
	appName := "udt-sensitive-app"
	appNamespace := "udt-sensitive-env-udt-sensitive-app"
	resourceTypeName := "Test.Resources/sensitiveResource"
	resourceName := "udt-sensitive-instance"
	filepath := "testdata/testresourcetypes.yaml"

	// Create step values
	createPassword := "super-secret-password"
	createAPIKey := "ak_1234567890abcdef"
	createCredentialSecret := "nested-secret-value"
	createConnectionConfigURL := "https://api.example.com"
	createConnectionConfigToken := "conn-token-abc123"

	// Update step values
	updatePassword := "updated-secret-password"
	updateAPIKey := "ak_updated_key_xyz"
	updateCredentialSecret := "updated-nested-secret"
	updateConnectionConfigURL := "https://api.example.com/v2"
	updateConnectionConfigToken := "conn-token-updated-xyz"

	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			// Step 1: Register the sensitiveResource type.
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				_, err := cli.ResourceProviderCreate(ctx, filepath)
				require.NoError(t, err)
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, test rp.RPTest) {
				output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceTypeName, "--output", "json"})
				require.NoError(t, err)
				require.Contains(t, output, resourceTypeName)
			},
		},
		{
			// Step 2: Deploy the resource with sensitive fields and verify redaction on GET and LIST.
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion(),
				"password="+createPassword, "apiKey="+createAPIKey,
				"credentialSecret="+createCredentialSecret,
				"connectionConfigUrl="+createConnectionConfigURL, "connectionConfigToken="+createConnectionConfigToken),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "udt-sensitive-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: resourceName,
						Type: resourceTypeName,
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				verifySensitiveFieldRedaction(ctx, t, ct, resourceTypeName, resourceName, appNamespace,
					createPassword, createAPIKey, createCredentialSecret, createConnectionConfigURL, createConnectionConfigToken)
			},
		},
		{
			// Step 3: Update the resource with new sensitive values and verify redaction still applies.
			Executor: step.NewDeployExecutor(template, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion(),
				"password="+updatePassword, "apiKey="+updateAPIKey,
				"credentialSecret="+updateCredentialSecret,
				"connectionConfigUrl="+updateConnectionConfigURL, "connectionConfigToken="+updateConnectionConfigToken),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "udt-sensitive-env",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: resourceName,
						Type: resourceTypeName,
					},
				},
			},
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			PostStepVerify: func(ctx context.Context, t *testing.T, ct rp.RPTest) {
				verifySensitiveFieldRedaction(ctx, t, ct, resourceTypeName, resourceName, appNamespace,
					updatePassword, updateAPIKey, updateCredentialSecret, updateConnectionConfigURL, updateConnectionConfigToken)
			},
		},
	})

	test.Test(t)
}

// verifySensitiveFieldRedaction verifies that sensitive fields are redacted on GET and LIST,
// non-sensitive fields are returned as plaintext, and the K8s Secret contains the expected decrypted values.
func verifySensitiveFieldRedaction(
	ctx context.Context,
	t *testing.T,
	ct rp.RPTest,
	resourceTypeName, resourceName, appNamespace string,
	expectedPassword, expectedAPIKey, expectedCredentialSecret, expectedConnectionConfigURL, expectedConnectionConfigToken string,
) {
	t.Helper()

	// --- GET verification ---
	resource, err := ct.Options.ManagementClient.GetResource(ctx, resourceTypeName, resourceName)
	require.NoError(t, err)
	require.NotNil(t, resource.Properties)

	// Non-sensitive fields MUST be returned as plaintext.
	require.Equal(t, "admin", resource.Properties["username"],
		"non-sensitive field 'username' should remain as plaintext")

	// Sensitive top-level fields MUST be null after backend redaction.
	require.Nil(t, resource.Properties["password"],
		"sensitive field 'password' should be null after redaction")
	require.Nil(t, resource.Properties["apiKey"],
		"sensitive field 'apiKey' should be null after redaction")

	// Nested fields: non-sensitive nested field should be intact, sensitive nested field should be null.
	credentials, ok := resource.Properties["credentials"].(map[string]any)
	require.True(t, ok, "credentials should be a map")
	require.Equal(t, "db.example.com", credentials["host"],
		"non-sensitive nested field 'credentials.host' should remain as plaintext")
	require.Nil(t, credentials["secret"],
		"nested sensitive field 'credentials.secret' should be null after redaction")

	// Sensitive object: entire connectionConfig should be null after redaction.
	require.Nil(t, resource.Properties["connectionConfig"],
		"sensitive object 'connectionConfig' should be null after redaction")

	// --- LIST verification ---
	resources, err := ct.Options.ManagementClient.ListResourcesOfType(ctx, resourceTypeName)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(resources), 1, "should find at least one sensitiveResource")

	found := false
	for _, res := range resources {
		if res.Name != nil && *res.Name == resourceName {
			found = true

			require.Equal(t, "admin", res.Properties["username"],
				"LIST: non-sensitive field 'username' should remain as plaintext")
			require.Nil(t, res.Properties["password"],
				"LIST: sensitive field 'password' should be null after redaction")
			require.Nil(t, res.Properties["apiKey"],
				"LIST: sensitive field 'apiKey' should be null after redaction")

			listCredentials, ok := res.Properties["credentials"].(map[string]any)
			require.True(t, ok, "LIST: credentials should be a map")
			require.Equal(t, "db.example.com", listCredentials["host"],
				"LIST: non-sensitive nested field 'credentials.host' should remain as plaintext")
			require.Nil(t, listCredentials["secret"],
				"LIST: nested sensitive field 'credentials.secret' should be null after redaction")

			require.Nil(t, res.Properties["connectionConfig"],
				"LIST: sensitive object 'connectionConfig' should be null after redaction")

			break
		}
	}
	require.True(t, found, "resource %s not found in LIST response", resourceName)

	// --- K8s Secret verification: prove decrypted values were passed to recipe ---
	secretName, ok := resource.Properties["secretName"].(string)
	require.True(t, ok, "recipe output 'secretName' should be a string")
	require.NotEmpty(t, secretName, "recipe output 'secretName' should not be empty")

	k8sSecret, err := ct.Options.K8sClient.CoreV1().Secrets(appNamespace).Get(ctx, secretName, metav1.GetOptions{})
	require.NoError(t, err, "should be able to read the K8s Secret created by the recipe")

	require.Equal(t, expectedPassword, string(k8sSecret.Data["password"]),
		"K8s Secret should contain the decrypted password")
	require.Equal(t, expectedAPIKey, string(k8sSecret.Data["apiKey"]),
		"K8s Secret should contain the decrypted apiKey")
	require.Equal(t, expectedCredentialSecret, string(k8sSecret.Data["secret"]),
		"K8s Secret should contain the decrypted nested secret")
	require.Equal(t, expectedConnectionConfigURL, string(k8sSecret.Data["connectionConfigUrl"]),
		"K8s Secret should contain the decrypted connectionConfig url")
	require.Equal(t, expectedConnectionConfigToken, string(k8sSecret.Data["connectionConfigToken"]),
		"K8s Secret should contain the decrypted connectionConfig token")
}
