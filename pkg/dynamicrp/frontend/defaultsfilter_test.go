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

package frontend

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	"github.com/radius-project/radius/pkg/armrpc/rest"
	"github.com/radius-project/radius/pkg/dynamicrp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestMakeDefaultsFilter_SchemaFetchError(t *testing.T) {
	ucpClient, err := testUCPClientFactoryWithError()
	require.NoError(t, err)

	resource := &datamodel.DynamicResource{
		Properties: map[string]any{"size": "L"},
	}

	response, err := makeDefaultsFilter(ucpClient)(createTestContext(), resource, nil, nil)
	require.NoError(t, err)

	errorResponse, ok := response.(*rest.InternalServerErrorResponse)
	require.True(t, ok)
	require.Equal(t, v1.CodeInternal, errorResponse.Body.Error.Code)
	require.Equal(t, "Failed to fetch schema to apply property defaults", errorResponse.Body.Error.Message)
	require.Equal(t, map[string]any{"size": "L"}, resource.Properties)
}

func TestMakeDefaultsFilter_NilProperties(t *testing.T) {
	t.Run("materializes declared defaults", func(t *testing.T) {
		ucpClient, err := createFakeUCPClientFactory(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"size": map[string]any{"type": "string", "default": "S"},
			},
		})
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{}
		response, err := makeDefaultsFilter(ucpClient)(createTestContext(), resource, nil, nil)
		require.NoError(t, err)
		require.Nil(t, response)
		require.Equal(t, map[string]any{"size": "S"}, resource.Properties)
	})

	t.Run("remains nil when no defaults apply", func(t *testing.T) {
		ucpClient, err := createFakeUCPClientFactory(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		})
		require.NoError(t, err)

		resource := &datamodel.DynamicResource{}
		response, err := makeDefaultsFilter(ucpClient)(createTestContext(), resource, nil, nil)
		require.NoError(t, err)
		require.Nil(t, response)
		require.Nil(t, resource.Properties)
	})
}

func TestMakeUpdateFilters_DefaultsBeforeEncryption(t *testing.T) {
	ucpClient, err := createFakeUCPClientFactory(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"password": map[string]any{
				"type":               "string",
				"default":            "default-password",
				"x-radius-sensitive": true,
			},
		},
	})
	require.NoError(t, err)

	resource := &datamodel.DynamicResource{}
	filters := makeUpdateFilters(
		makeDefaultsFilter(ucpClient),
		makeEncryptionFilter(ucpClient, createTestHandler(t)),
	)
	for _, filter := range filters {
		response, err := filter(createTestContext(), resource, nil, nil)
		require.NoError(t, err)
		require.Nil(t, response)
	}

	encryptedPassword, ok := resource.Properties["password"].(map[string]any)
	require.True(t, ok, "defaulted password should be encrypted")
	require.Contains(t, encryptedPassword, "encrypted")
	require.Contains(t, encryptedPassword, "nonce")
	require.Contains(t, encryptedPassword, "version")
}
