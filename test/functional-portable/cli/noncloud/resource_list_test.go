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
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/ucp/resources"
	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/step"
	"github.com/radius-project/radius/test/testutil"
	"github.com/radius-project/radius/test/validation"
	"github.com/stretchr/testify/require"
)

// Test_ResourceList tests the rad resource list command with various filters
func Test_ResourceList(t *testing.T) {
	template := "testdata/corerp-resource-list-test.bicep"
	appName := "resource-list-test"

	test := rp.NewRPTest(t, appName, []rp.TestStep{
		{
			Executor: step.NewDeployExecutor(template, testutil.GetMagpieImage()),
			RPResources: &validation.RPResourceSet{
				Resources: []validation.RPResource{
					{
						Name: "test-environment",
						Type: validation.EnvironmentsResource,
					},
					{
						Name: appName,
						Type: validation.ApplicationsResource,
					},
					{
						Name: "containerA",
						Type: validation.ContainersResource,
						App:  appName,
					},
					{
						Name: "containerB",
						Type: validation.ContainersResource,
						App:  appName,
					},
					{
						Name: "test-secretstore",
						Type: validation.SecretStoresResource,
						App:  appName,
					},
				},
			},
			K8sObjects: &validation.K8sObjectSet{
				Namespaces: map[string][]validation.K8sObject{
					"test-environment-resource-list-test": {
						validation.NewK8sPodForResource(appName, "containera"),
						validation.NewK8sPodForResource(appName, "containerb"),
					},
				},
			},
			PostStepVerify: verifyResourceListCommands,
		},
	})

	test.Test(t)
}

// verifyResourceListCommands tests various resource list scenarios using table-driven tests
func verifyResourceListCommands(ctx context.Context, t *testing.T, test rp.RPTest) {
	options := rp.NewRPTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	scope, err := resources.ParseScope(options.Workspace.Scope)
	require.NoError(t, err)

	groupName := scope.Name()
	appName := test.Name          // "resource-list-test"
	envName := "test-environment" // Now this exists in the deployment!

	// Table-driven tests - simple and effective
	testCases := []struct {
		name            string
		resourceType    string
		groupName       string
		environmentName string
		applicationName string
		expectError     bool
		errorContains   string
		validateOutput  func(t *testing.T, output string)
	}{
		// Positive test cases - single filters
		{
			name:            "list containers by application",
			resourceType:    "Applications.Core/containers",
			applicationName: appName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},
		{
			name:         "list containers by group",
			resourceType: "Applications.Core/containers",
			groupName:    groupName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},
		{
			name:            "list containers by environment",
			resourceType:    "Applications.Core/containers",
			environmentName: envName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},

		// Combined filters
		{
			name:            "list with all filters",
			resourceType:    "Applications.Core/containers",
			groupName:       groupName,
			environmentName: envName,
			applicationName: appName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},
		{
			name:            "list with group and environment filters",
			resourceType:    "Applications.Core/containers",
			groupName:       groupName,
			environmentName: envName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},

		// List all resources
		{
			name:      "list all resources in group (no type specified)",
			groupName: groupName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
				require.Contains(t, output, "test-secretstore")
				require.Contains(t, output, appName)
				require.Contains(t, output, "test-environment")
				require.Contains(t, output, "Applications.Core/containers")
				require.Contains(t, output, "Applications.Core/applications")
				require.Contains(t, output, "Applications.Core/secretStores")
				require.Contains(t, output, "Applications.Core/environments")
			},
		},
		{
			name:         "list only applications",
			resourceType: "Applications.Core/applications",
			groupName:    groupName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, appName)
				require.NotContains(t, output, "containerA")
				require.NotContains(t, output, "containerB")
			},
		},

		// Test different resource types
		{
			name:            "list secretStores by application",
			resourceType:    "Applications.Core/secretStores",
			applicationName: appName,
			validateOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "test-secretstore")
				require.NotContains(t, output, "containerA")
			},
		},

		// Edge case - empty result
		{
			name:         "empty result for non-existent resource type",
			resourceType: "Applications.Core/volumes",
			groupName:    groupName,
			validateOutput: func(t *testing.T, output string) {
				lines := strings.Split(strings.TrimSpace(output), "\n")
				require.Equal(t, 1, len(lines), "Should only have header for empty results")
				require.Contains(t, lines[0], "RESOURCE")
			},
		},

		// List all resources (no type, no filters - should default to workspace's active group)
		{
			name: "list all resources with no filters (defaults to workspace's active group)",
			validateOutput: func(t *testing.T, output string) {
				// Should list all resources in the workspace's default group
				require.Contains(t, output, "Applications.Core/containers")
				require.Contains(t, output, "Applications.Core/applications")
				require.Contains(t, output, "containerA")
				require.Contains(t, output, "containerB")
			},
		},

		// Error cases with fixed error messages
		{
			name:            "error with non-existent application",
			resourceType:    "Applications.Core/containers",
			applicationName: "non-existent-app",
			expectError:     true,
			errorContains:   "could not be found",
		},
		{
			name:            "error with non-existent environment",
			resourceType:    "Applications.Core/containers",
			environmentName: "non-existent-env",
			expectError:     true,
			errorContains:   "could not be found",
		},
		{
			name:          "error with non-existent group",
			resourceType:  "Applications.Core/containers",
			groupName:     "non-existent-group",
			expectError:   true,
			errorContains: "was not found",
		},
	}

	// Execute tests - simple and direct
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := cli.ResourceListWithFilters(ctx,
				tc.resourceType, tc.groupName, tc.environmentName, tc.applicationName)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					// Check the actual CLI output for error messages, not the wrapped error
					require.Contains(t, output, tc.errorContains)
				}
			} else {
				require.NoError(t, err)
				if tc.validateOutput != nil {
					tc.validateOutput(t, output)
				}
			}
		})
	}
}
