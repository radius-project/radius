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
)

// Test_DynamicRP_SensitiveFieldEncryption tests that fields marked with x-radius-sensitive annotation
// in a resource type schema are encrypted on PUT, redacted (nullified) by the backend after recipe
// execution, and returned as null on GET/LIST.
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
//   - Verifies via GET that sensitive fields are null (redacted by backend)
//   - Verifies via LIST that the same redaction behavior applies
//
// 3. Resource Update:
//   - Deploys an updated Bicep template with different sensitive values
//   - Verifies that sensitive fields are null after redaction
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
			// Step 2: Deploy the resource with sensitive fields and verify redaction on GET and LIST.
			Executor: step.NewDeployExecutor(createTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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

						// Sensitive fields in LIST response MUST be null after redaction.
						require.Nil(t, res.Properties["password"],
							"LIST: sensitive field 'password' should be null after redaction")
						require.Nil(t, res.Properties["apiKey"],
							"LIST: sensitive field 'apiKey' should be null after redaction")

						// Nested sensitive field in LIST response.
						listCredentials, ok := res.Properties["credentials"].(map[string]any)
						require.True(t, ok, "LIST: credentials should be a map")
						require.Equal(t, "db.example.com", listCredentials["host"],
							"LIST: non-sensitive nested field 'credentials.host' should remain as plaintext")
						require.Nil(t, listCredentials["secret"],
							"LIST: nested sensitive field 'credentials.secret' should be null after redaction")

						break
					}
				}
				require.True(t, found, "resource %s not found in LIST response", resourceName)
			},
		},
		{
			// Step 3: Update the resource with new sensitive values and verify redaction still applies.
			Executor: step.NewDeployExecutor(updateTemplate, testutil.GetBicepRecipeRegistry(), testutil.GetBicepRecipeVersion()),
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

				// Sensitive fields must be null after redaction (both old and new values redacted).
				require.Nil(t, resource.Properties["password"],
					"password should be null after redaction on update")
				require.Nil(t, resource.Properties["apiKey"],
					"apiKey should be null after redaction on update")

				// Nested fields after update.
				credentials, ok := resource.Properties["credentials"].(map[string]any)
				require.True(t, ok, "credentials should be a map")

				require.Equal(t, "db.example.com", credentials["host"],
					"non-sensitive nested field should be intact after update")
				require.Nil(t, credentials["secret"],
					"nested secret should be null after redaction on update")

				// --- LIST verification after update ---
				resources, err := ct.Options.ManagementClient.ListResourcesOfType(ctx, resourceTypeName)
				require.NoError(t, err)

				found := false
				for _, res := range resources {
					if res.Name != nil && *res.Name == resourceName {
						found = true

						require.Equal(t, "admin", res.Properties["username"],
							"LIST after update: non-sensitive field 'username' should remain as plaintext")
						require.Nil(t, res.Properties["password"],
							"LIST after update: sensitive field 'password' should be null after redaction")
						require.Nil(t, res.Properties["apiKey"],
							"LIST after update: sensitive field 'apiKey' should be null after redaction")

						listCredentials, ok := res.Properties["credentials"].(map[string]any)
						require.True(t, ok, "LIST after update: credentials should be a map")
						require.Equal(t, "db.example.com", listCredentials["host"],
							"LIST after update: non-sensitive nested field 'credentials.host' should remain as plaintext")
						require.Nil(t, listCredentials["secret"],
							"LIST after update: nested sensitive field 'credentials.secret' should be null after redaction")

						break
					}
				}
				require.True(t, found, "resource %s not found in LIST response after update", resourceName)
			},
		},
	})

	test.Test(t)
}
