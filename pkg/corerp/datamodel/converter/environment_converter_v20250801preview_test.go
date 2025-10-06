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

package converter

import (
	"encoding/json"
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	v20250801preview "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"github.com/radius-project/radius/pkg/corerp/datamodel"
	"github.com/stretchr/testify/require"
)

func TestEnvironment20250801DataModelToVersioned(t *testing.T) {
	testset := []struct {
		name          string
		dataModelFile string
		apiVersion    string
		expectedType  any
		expectError   error
	}{
		{
			name:          "Azure identity environment",
			dataModelFile: "../../api/v20250801preview/testdata/environmentresourcedatamodel-azure-identity.json",
			apiVersion:    v20250801preview.Version,
			expectedType:  &v20250801preview.EnvironmentResource{},
			expectError:   nil,
		},
		{
			name:          "Kubernetes environment",
			dataModelFile: "../../api/v20250801preview/testdata/environmentresourcedatamodel-kubernetes.json",
			apiVersion:    v20250801preview.Version,
			expectedType:  &v20250801preview.EnvironmentResource{},
			expectError:   nil,
		},
		{
			name:          "Simulated environment",
			dataModelFile: "../../api/v20250801preview/testdata/environmentresourcedatamodel-simulated.json",
			apiVersion:    v20250801preview.Version,
			expectedType:  &v20250801preview.EnvironmentResource{},
			expectError:   nil,
		},
		{
			name:          "Unsupported API version",
			dataModelFile: "",
			apiVersion:    "unsupported-version",
			expectedType:  nil,
			expectError:   v1.ErrUnsupportedAPIVersion,
		},
	}

	for _, tc := range testset {
		t.Run(tc.name, func(t *testing.T) {
			var dm *datamodel.Environment_v20250801preview

			if tc.dataModelFile != "" {
				content := loadTestData(tc.dataModelFile)
				require.NotNil(t, content, "Failed to load test data file: %s", tc.dataModelFile)

				dm = &datamodel.Environment_v20250801preview{}
				err := json.Unmarshal(content, dm)
				require.NoError(t, err, "Failed to unmarshal test data")
			}

			result, err := Environment20250801DataModelToVersioned(dm, tc.apiVersion)

			if tc.expectError != nil {
				require.Error(t, err)
				require.ErrorAs(t, err, &tc.expectError)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, tc.expectedType, result)

				// Verify the converted result is a proper versioned model
				require.NotNil(t, result)
			}
		})
	}
}

func TestEnvironment20250801DataModelFromVersioned(t *testing.T) {
	testset := []struct {
		name               string
		versionedModelFile string
		apiVersion         string
		expectError        error
	}{
		{
			name:               "Azure identity environment",
			versionedModelFile: "../../api/v20250801preview/testdata/environmentresource-azure-identity.json",
			apiVersion:         v20250801preview.Version,
			expectError:        nil,
		},
		{
			name:               "Kubernetes environment",
			versionedModelFile: "../../api/v20250801preview/testdata/environmentresource-kubernetes.json",
			apiVersion:         v20250801preview.Version,
			expectError:        nil,
		},
		{
			name:               "Hybrid environment",
			versionedModelFile: "../../api/v20250801preview/testdata/environmentresource-hybrid.json",
			apiVersion:         v20250801preview.Version,
			expectError:        nil,
		},
		{
			name:               "Unsupported API version",
			versionedModelFile: "",
			apiVersion:         "unsupported-version",
			expectError:        v1.ErrUnsupportedAPIVersion,
		},
		{
			name:               "Invalid JSON",
			versionedModelFile: "",
			apiVersion:         v20250801preview.Version,
			expectError:        nil, // Will be a JSON unmarshal error
		},
	}

	for _, tc := range testset {
		t.Run(tc.name, func(t *testing.T) {
			var content []byte

			if tc.versionedModelFile != "" {
				content = loadTestData(tc.versionedModelFile)
				require.NotNil(t, content, "Failed to load test data file: %s", tc.versionedModelFile)
			} else if tc.name == "Invalid JSON" {
				content = []byte(`{"invalid": json}`)
			}

			result, err := Environment20250801DataModelFromVersioned(content, tc.apiVersion)

			if tc.expectError != nil {
				require.Error(t, err)
				if tc.expectError == v1.ErrUnsupportedAPIVersion {
					require.ErrorAs(t, err, &tc.expectError)
				}
				require.Nil(t, result)
			} else if tc.name == "Invalid JSON" {
				require.Error(t, err)
				require.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.IsType(t, &datamodel.Environment_v20250801preview{}, result)

				// Verify the API version is properly set
				require.Equal(t, tc.apiVersion, result.InternalMetadata.UpdatedAPIVersion)
			}
		})
	}
}

func TestEnvironment20250801DataModelEdgeCases(t *testing.T) {
	t.Run("Empty datamodel to versioned", func(t *testing.T) {
		emptyDM := &datamodel.Environment_v20250801preview{}
		result, err := Environment20250801DataModelToVersioned(emptyDM, v20250801preview.Version)
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should create a versioned model with empty properties
		envResource, ok := result.(*v20250801preview.EnvironmentResource)
		require.True(t, ok)
		require.NotNil(t, envResource)
		require.NotNil(t, envResource.Properties)
	})

	t.Run("Minimal JSON from versioned", func(t *testing.T) {
		minimalJSON := []byte(`{"properties": {}}`)
		result, err := Environment20250801DataModelFromVersioned(minimalJSON, v20250801preview.Version)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.IsType(t, &datamodel.Environment_v20250801preview{}, result)
	})

	t.Run("Nil content from versioned", func(t *testing.T) {
		result, err := Environment20250801DataModelFromVersioned(nil, v20250801preview.Version)
		require.Error(t, err) // Should fail on JSON unmarshal
		require.Nil(t, result)
	})
}
