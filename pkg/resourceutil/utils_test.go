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

package resourceutil

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
)

const (
	TestResourceType  = "Applications.Test/testResources"
	TestEnvironmentID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/environments/test-env"
	TestApplicationID = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Core/applications/test-app"
	TestResourceID    = "/planes/radius/local/resourceGroups/radius-test-rg/providers/Applications.Test/testResources/tr"
)

type PropertiesTestResource struct {
	v1.BaseResource
	Properties map[string]any `json:"properties"`
}

func (p *PropertiesTestResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return nil
}

func (p *PropertiesTestResource) ApplyDeploymentOutput(deploymentOutput rpv1.DeploymentOutput) error {
	return nil
}

func (p *PropertiesTestResource) OutputResources() []rpv1.OutputResource {
	return nil
}

func TestGetPropertiesFromResource(t *testing.T) {
	tests := []struct {
		name        string
		resource    *PropertiesTestResource
		expected    map[string]any
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid properties",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"Application": TestApplicationID,
					"Environment": TestEnvironmentID,
				},
			},
			expected: map[string]any{
				"Application": TestApplicationID,
				"Environment": TestEnvironmentID,
			},
			expectError: false,
		},
		{
			name: "Empty properties",
			resource: &PropertiesTestResource{
				Properties: nil,
			},
			expected:    map[string]any{},
			expectError: false,
		},
		{
			name: "Invalid JSON",
			resource: &PropertiesTestResource{
				Properties: map[string]any{
					"key": func() {}, // Functions cannot be marshaled to JSON
				},
			},
			expected:    nil,
			expectError: true,
			errorMsg:    errMarshalResource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties, err := GetPropertiesFromResource(tt.resource)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, properties)
				require.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, properties)
				require.Equal(t, tt.expected, properties)
			}
		})
	}
}

// InvalidTestResource is a test resource with invalid properties type.
type InvalidTestResource struct {
	v1.BaseResource
	Name string `json:"properties"`
}

func (p *InvalidTestResource) ResourceMetadata() rpv1.BasicResourcePropertiesAdapter {
	return nil
}

func (p *InvalidTestResource) ApplyDeploymentOutput(deploymentOutput rpv1.DeploymentOutput) error {
	return nil
}

func (p *InvalidTestResource) OutputResources() []rpv1.OutputResource {
	return nil
}

func TestGetPropertiesFromResource_MissingProperties(t *testing.T) {
	testResource := &InvalidTestResource{
		Name: "test-resource",
	}

	properties, err := GetPropertiesFromResource(testResource)
	require.Error(t, err)
	require.Nil(t, properties)
	require.Contains(t, err.Error(), errUnmarshalResourceProperties)
}
