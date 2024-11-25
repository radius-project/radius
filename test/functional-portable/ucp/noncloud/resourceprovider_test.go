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

package ucp

import (
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/test/radcli"
	"github.com/radius-project/radius/test/rp"
	"github.com/radius-project/radius/test/testcontext"
	"github.com/stretchr/testify/require"
)

func Test_ResourceProviderRegistration(t *testing.T) {
	ctx, cancel := testcontext.NewWithCancel(t)
	t.Cleanup(cancel)

	options := rp.NewTestOptions(t)
	cli := radcli.NewCLI(t, options.ConfigFilePath)

	// Setup test data.
	const (
		manifestPath             = "testdata/resourceprovider.yaml"
		resourceProviderName     = "MyCompany.Resources"
		expectedResourceTypeName = "testResources"
		expectedApiVersion       = "2025-01-01-preview"
	)

	expectedData := map[string]any{
		"name": resourceProviderName,
		"locations": map[string]any{
			"global": map[string]any{},
		},
		"resourceTypes": map[string]any{
			expectedResourceTypeName: map[string]any{
				"apiVersions": map[string]any{
					expectedApiVersion: map[string]any{},
				},
			},
		},
	}

	// Create the resource provider using the manifest.
	_, err := cli.RunCommand(ctx, []string{"resource-provider", "create", "--from-file", manifestPath})
	require.NoError(t, err)

	// List resource providers.
	output, err := cli.RunCommand(ctx, []string{"resource-provider", "list"})
	require.NoError(t, err)
	require.Contains(t, output, resourceProviderName)

	// Show details of the resource provider.
	output, err = cli.RunCommand(ctx, []string{"resource-provider", "show", resourceProviderName, "--output", "json"})
	require.NoError(t, err)

	var data map[string]any
	err = json.Unmarshal([]byte(output), &data)
	require.NoError(t, err)
	require.Equal(t, expectedData, data)
}
