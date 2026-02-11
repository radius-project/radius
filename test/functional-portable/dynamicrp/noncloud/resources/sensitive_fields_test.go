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
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

// Test_DynamicRP_SensitiveFieldEncryption tests that fields marked with x-radius-sensitive annotation
// in a resource type schema are encrypted on PUT and not returned as plaintext on GET/LIST.
//
// The test consists of three main steps:
//
// 1. Resource Type Registration:
//   - Registers a user-defined resource type "Test.Resources/sensitiveResource" with ManualResourceProvisioning
//   - The schema includes sensitive fields (password, apiKey, credentials.secret) and non-sensitive fields (username, credentials.host)
//
// 2. Resource Deployment (Create):
//   - Deploys a Bicep template that creates a sensitiveResource instance with plaintext sensitive values
//   - Verifies via GET that non-sensitive fields are returned as plaintext
//   - Verifies via GET that sensitive fields are NOT returned as plaintext (encrypted on PUT)
//   - Verifies via LIST that the same encryption behavior applies
//
// 3. Resource Update:
//   - Deploys an updated Bicep template with different sensitive values
//   - Verifies that neither the old nor new plaintext values are exposed
func Test_DynamicRP_SensitiveFieldEncryption(t *testing.T) {
	createTemplate := "testdata/sensitive-resource.bicep"
	updateTemplate := "testdata/sensitive-resource-update.bicep"
	appName := "udt-sensitive-app"
	resourceTypeName := "Test.Resources/sensitiveResource"
	resourceName := "udt-sensitive-instance"
	filepath := "testdata/testresourcetypes.yaml"
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
			// Step 2: Deploy the resource with sensitive fields and verify encryption on GET and LIST.
			Executor: step.NewDeployExecutor(createTemplate),
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
				// --- GET verification ---
				resource, err := ct.Options.ManagementClient.GetResource(ctx, resourceTypeName, resourceName)
				require.NoError(t, err)
				require.NotNil(t, resource.Properties)

				// Non-sensitive fields MUST be returned as plaintext.
				require.Equal(t, "admin", resource.Properties["username"],
					"non-sensitive field 'username' should remain as plaintext")

				// Sensitive top-level fields MUST NOT be returned as plaintext.
				password := resource.Properties["password"]
				require.NotEqual(t, "super-secret-password", password,
					"sensitive field 'password' must not be returned as plaintext")

				apiKey := resource.Properties["apiKey"]
				require.NotEqual(t, "ak_1234567890abcdef", apiKey,
					"sensitive field 'apiKey' must not be returned as plaintext")

				// Verify password is in encrypted format (map with "encrypted", "nonce" keys).
				// This confirms the encryption filter ran on PUT.
				passwordMap, isMap := password.(map[string]any)
				if isMap {
					require.Contains(t, passwordMap, "encrypted",
						"encrypted password should contain 'encrypted' key")
					require.Contains(t, passwordMap, "nonce",
						"encrypted password should contain 'nonce' key")
					t.Log("password is in encrypted map format (pre-redaction behavior)")
				}

				// TODO: Uncomment when GET/LIST redaction is implemented.
				// require.Nil(t, password, "sensitive field 'password' should be null after redaction")
				// require.Nil(t, apiKey, "sensitive field 'apiKey' should be null after redaction")

				// Nested fields: non-sensitive nested field should be intact, sensitive nested field should be encrypted.
				credentials, ok := resource.Properties["credentials"].(map[string]any)
				require.True(t, ok, "credentials should be a map")

				require.Equal(t, "db.example.com", credentials["host"],
					"non-sensitive nested field 'credentials.host' should remain as plaintext")

				secret := credentials["secret"]
				require.NotEqual(t, "nested-secret-value", secret,
					"nested sensitive field 'credentials.secret' must not be returned as plaintext")

				// TODO: Uncomment when GET/LIST redaction is implemented.
				// require.Nil(t, secret, "nested sensitive field 'credentials.secret' should be null after redaction")

				// --- LIST verification ---
				resources, err := ct.Options.ManagementClient.ListResourcesOfType(ctx, resourceTypeName)
				require.NoError(t, err)
				require.GreaterOrEqual(t, len(resources), 1, "should find at least one sensitiveResource")

				found := false
				for _, res := range resources {
					if res.Name != nil && *res.Name == resourceName {
						found = true

						// Non-sensitive field in LIST response.
						require.Equal(t, "admin", res.Properties["username"],
							"LIST: non-sensitive field 'username' should remain as plaintext")

						// Sensitive field in LIST response MUST NOT be plaintext.
						listPassword := res.Properties["password"]
						require.NotEqual(t, "super-secret-password", listPassword,
							"LIST: sensitive field 'password' must not be returned as plaintext")

						listApiKey := res.Properties["apiKey"]
						require.NotEqual(t, "ak_1234567890abcdef", listApiKey,
							"LIST: sensitive field 'apiKey' must not be returned as plaintext")

						// TODO: Uncomment when GET/LIST redaction is implemented.
						// require.Nil(t, listPassword, "LIST: sensitive field 'password' should be null after redaction")
						// require.Nil(t, listApiKey, "LIST: sensitive field 'apiKey' should be null after redaction")

						break
					}
				}
				require.True(t, found, "resource %s not found in LIST response", resourceName)
			},
		},
		{
			// Step 3: Update the resource with new sensitive values and verify encryption still applies.
			Executor: step.NewDeployExecutor(updateTemplate),
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
				resource, err := ct.Options.ManagementClient.GetResource(ctx, resourceTypeName, resourceName)
				require.NoError(t, err)
				require.NotNil(t, resource.Properties)

				// Non-sensitive field unchanged after update.
				require.Equal(t, "admin", resource.Properties["username"],
					"non-sensitive field 'username' should remain as plaintext after update")

				// Updated sensitive fields must not be the old or new plaintext.
				password := resource.Properties["password"]
				require.NotEqual(t, "super-secret-password", password,
					"password must not be the OLD plaintext after update")
				require.NotEqual(t, "updated-secret-password", password,
					"password must not be the NEW plaintext after update")

				apiKey := resource.Properties["apiKey"]
				require.NotEqual(t, "ak_1234567890abcdef", apiKey,
					"apiKey must not be the OLD plaintext after update")
				require.NotEqual(t, "ak_updated_key_xyz", apiKey,
					"apiKey must not be the NEW plaintext after update")

				// Nested fields after update.
				credentials, ok := resource.Properties["credentials"].(map[string]any)
				require.True(t, ok, "credentials should be a map")

				require.Equal(t, "db.example.com", credentials["host"],
					"non-sensitive nested field should be intact after update")

				secret := credentials["secret"]
				require.NotEqual(t, "nested-secret-value", secret,
					"nested secret must not be the OLD plaintext after update")
				require.NotEqual(t, "updated-nested-secret", secret,
					"nested secret must not be the NEW plaintext after update")

				// TODO: Uncomment when GET/LIST redaction is implemented.
				// require.Nil(t, password, "password should be null after redaction")
				// require.Nil(t, apiKey, "apiKey should be null after redaction")
				// require.Nil(t, secret, "nested secret should be null after redaction")
			},
		},
	})

	test.Test(t)
}
