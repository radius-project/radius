package controller

import (
	"testing"

	v1 "github.com/radius-project/radius/pkg/armrpc/api/v1"
	rpv1 "github.com/radius-project/radius/pkg/rp/v1"
	"github.com/stretchr/testify/require"
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
