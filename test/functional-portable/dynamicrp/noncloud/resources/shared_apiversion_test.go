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
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/test"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/stretchr/testify/require"
)

// Test_DynamicRP_SharedAPIVersionDeletion validates that when deleting a resource type that shares an API version
// with other resource types, the API versions of other resource types are not affected.
//
// The test consists of two main steps:
//
// 1. Resource Type Registration:
//   - Registers multiple resource types that share the same API version (2023-10-01-preview)
//   - Verifies that all resource types have the shared API version in both individual and summary views
//
// 2. Resource Type Deletion and Validation:
//   - Deletes one resource type (sharedAPITestTypeA)
//   - Verifies that other resource types (sharedAPITestTypeB) still retain their API versions
//   - Validates that the deleted resource type is properly removed from the provider summary
//
// This test prevents regression of the bug where deleting one resource type
// would cause API versions to disappear from other unrelated resource types.
func Test_DynamicRP_SharedAPIVersionDeletion(t *testing.T) {
	filepath := "testdata/testresourcetypes.yaml"
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Resource types that share the same API version (2023-10-01-preview)
	resourceProviderName := "Test.Resources"
	resourceTypesToDelete := "Test.Resources/sharedAPITestTypeA"
	resourceTypesToPreserve := []string{
		"Test.Resources/sharedAPITestTypeB",
	}
	sharedAPIVersion := "2023-10-01-preview"

	test := rp.NewRPTest(t, "shared-apiversion-test", []rp.TestStep{
		{
			// Step 1: Register all resource types including shared API versions
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				// Create the resource provider with multiple resource types
				_, err := cli.ResourceProviderCreate(ctx, filepath)
				require.NoError(t, err)

				// Verify that all resource types were created and have the shared API version
				for _, resourceType := range append(resourceTypesToPreserve, resourceTypesToDelete) {
					t.Logf("Verifying initial state of resource type: %s", resourceType)
					output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceType, "--output", "json"})
					require.NoError(t, err)

					// Parse the JSON output to verify API version exists
					t.Logf("CLI output for resource type %s: %s", resourceType, output)
					var resourceTypeData map[string]any
					err = json.Unmarshal([]byte(output), &resourceTypeData)
					require.NoError(t, err)

					require.True(t, hasAPIVersion(t, resourceTypeData, sharedAPIVersion), "Resource type %s should have API version %s", resourceType, sharedAPIVersion)
				}

				// Verify resource provider summary shows all resource types with API versions
				output, err := cli.RunCommand(ctx, []string{"resource-provider", "show", resourceProviderName, "--output", "json"})
				require.NoError(t, err)

				var providerData map[string]any
				err = json.Unmarshal([]byte(output), &providerData)
				require.NoError(t, err)

				resourceTypes, ok := providerData["resourceTypes"].(map[string]any)
				require.True(t, ok, "resourceTypes should exist in resource provider")

				// Verify each resource type has the shared API version in the summary
				for _, resourceType := range append(resourceTypesToPreserve, resourceTypesToDelete) {
					resourceTypeName := resourceType[len(resourceProviderName)+1:] // Remove "Test.Resources/" prefix
					resourceTypeEntry, ok := resourceTypes[resourceTypeName].(map[string]any)
					require.True(t, ok, "Resource type %s should exist in provider summary", resourceTypeName)

					apiVersions, ok := resourceTypeEntry["apiVersions"].(map[string]any)
					require.True(t, ok, "apiVersions should exist for resource type %s", resourceTypeName)

					_, hasSharedVersion := apiVersions[sharedAPIVersion]
					require.True(t, hasSharedVersion, "Resource type %s should have API version %s in provider summary", resourceTypeName, sharedAPIVersion)
				}
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
		{
			// Step 2: Delete one resource type and verify others are not affected
			Executor: step.NewFuncExecutor(func(ctx context.Context, t *testing.T, options test.TestOptions) {
				t.Logf("Deleting resource type: %s", resourceTypesToDelete)

				// Delete the postgres resource type
				_, err := cli.RunCommand(ctx, []string{"resource-type", "delete", resourceTypesToDelete, "--yes"})
				require.NoError(t, err)

				// Verify the deleted resource type is gone
				_, err = cli.RunCommand(ctx, []string{"resource-type", "show", resourceTypesToDelete, "--output", "json"})
				require.Error(t, err, "Deleted resource type should not be found")

				// CRITICAL: Verify that other resource types still have their API versions
				for _, resourceType := range resourceTypesToPreserve {
					t.Logf("Verifying preserved resource type: %s", resourceType)
					output, err := cli.RunCommand(ctx, []string{"resource-type", "show", resourceType, "--output", "json"})
					require.NoError(t, err, "Resource type %s should still exist after deleting %s", resourceType, resourceTypesToDelete)

					// Parse the JSON output to verify API version still exists
					t.Logf("CLI output for preserved resource type %s: %s", resourceType, output)
					var resourceTypeData map[string]interface{}
					err = json.Unmarshal([]byte(output), &resourceTypeData)
					require.NoError(t, err)

					require.True(t, hasAPIVersion(t, resourceTypeData, sharedAPIVersion), "BUG: Resource type %s lost API version %s after deleting %s - this is the bug we're fixing!", resourceType, sharedAPIVersion, resourceTypesToDelete)
				}

				// Verify resource provider summary still shows preserved resource types with API versions
				output, err := cli.RunCommand(ctx, []string{"resource-provider", "show", resourceProviderName, "--output", "json"})
				require.NoError(t, err)

				var providerData map[string]interface{}
				err = json.Unmarshal([]byte(output), &providerData)
				require.NoError(t, err)

				resourceTypes, ok := providerData["resourceTypes"].(map[string]interface{})
				require.True(t, ok, "resourceTypes should exist in resource provider after deletion")

				// Verify deleted resource type is removed from summary
				deletedResourceTypeName := resourceTypesToDelete[len(resourceProviderName)+1:] // Remove "Test.Resources/" prefix
				_, exists := resourceTypes[deletedResourceTypeName]
				require.False(t, exists, "Deleted resource type %s should not exist in provider summary", deletedResourceTypeName)

				// Verify preserved resource types still have API versions in summary
				for _, resourceType := range resourceTypesToPreserve {
					resourceTypeName := resourceType[len(resourceProviderName)+1:] // Remove "Test.Resources/" prefix
					resourceTypeEntry, ok := resourceTypes[resourceTypeName].(map[string]interface{})
					require.True(t, ok, "Preserved resource type %s should exist in provider summary", resourceTypeName)

					apiVersions, ok := resourceTypeEntry["apiVersions"].(map[string]interface{})
					require.True(t, ok, "apiVersions should exist for preserved resource type %s", resourceTypeName)

					_, hasSharedVersion := apiVersions[sharedAPIVersion]
					require.True(t, hasSharedVersion, "BUG: Resource type %s lost API version %s in provider summary after deleting %s - this is the bug we're fixing!", resourceTypeName, sharedAPIVersion, resourceTypesToDelete)
				}

				t.Logf("✅ SUCCESS: Shared API version deletion test passed - preserved resource types retained their API versions")
			}),
			SkipKubernetesOutputResourceValidation: true,
			SkipObjectValidation:                   true,
			SkipResourceDeletion:                   true,
		},
	})

	test.Test(t)
}

// hasAPIVersion checks if a resource type contains the specified API version.
func hasAPIVersion(t *testing.T, resourceTypeData map[string]any, expectedVersion string) bool {
	t.Logf("APIVersions field type: %T, value: %+v", resourceTypeData["APIVersions"], resourceTypeData["APIVersions"])

	switch apiVersions := resourceTypeData["APIVersions"].(type) {
	case map[string]any:
		// APIVersions as map {"2023-10-01-preview": {...}}
		_, exists := apiVersions[expectedVersion]
		return exists
	case []any:
		// APIVersions as array ["2023-10-01-preview"]
		for _, version := range apiVersions {
			if version.(string) == expectedVersion {
				return true
			}
		}
		return false
	default:
		t.Fatalf("APIVersions has unexpected type %T, expected map or array. Full data: %+v", apiVersions, resourceTypeData)
		return false
	}
}
