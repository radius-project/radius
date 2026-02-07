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

package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewValidateAppDefinitionSkill(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()
	require.NotNil(t, skill)
	assert.Equal(t, "validate_app_definition", skill.Name())
}

func TestValidateAppDefinitionSkill_Execute_ValidBicep(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	validBicep := `extension radius

@description('The Radius environment to deploy to')
param environment string

@description('The application name')
param applicationName string = 'testapp'

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: applicationName
  properties: {
    environment: environment
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: validBicep,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.True(t, output.Valid)
	assert.Empty(t, filterErrors(output.Issues))
}

func TestValidateAppDefinitionSkill_Execute_MissingExtension(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	invalidBicep := `
param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: invalidBicep,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.False(t, output.Valid)
	errors := filterErrors(output.Issues)
	assert.Len(t, errors, 1)
	assert.Contains(t, errors[0].Message, "extension radius")
}

func TestValidateAppDefinitionSkill_Execute_MissingEnvironmentParam(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	invalidBicep := `extension radius

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: 'default'
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: invalidBicep,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.False(t, output.Valid)
	errors := filterErrors(output.Issues)
	hasEnvError := false
	for _, e := range errors {
		if e.Message == "missing 'environment' parameter" {
			hasEnvError = true
			break
		}
	}
	assert.True(t, hasEnvError)
}

func TestValidateAppDefinitionSkill_Execute_MissingAppResource(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	invalidBicep := `extension radius

param environment string

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mycontainer'
  properties: {
    environment: environment
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: invalidBicep,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.False(t, output.Valid)
	errors := filterErrors(output.Issues)
	hasAppError := false
	for _, e := range errors {
		if e.Message == "missing application resource definition" {
			hasAppError = true
			break
		}
	}
	assert.True(t, hasAppError)
}

func TestValidateAppDefinitionSkill_Execute_TODOWarnings(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	bicepWithTODO := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mycontainer'
  properties: {
    container: {
      image: 'myimage:latest' // TODO: Update with actual image
    }
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: bicepWithTODO,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	warnings := filterWarnings(output.Issues)
	assert.GreaterOrEqual(t, len(warnings), 1)
	hasTODOWarning := false
	for _, w := range warnings {
		if w.Message == "found 1 TODO comments that need attention" {
			hasTODOWarning = true
			break
		}
	}
	assert.True(t, hasTODOWarning)
}

func TestValidateAppDefinitionSkill_Execute_StrictMode(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	bicepWithLatest := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}

resource container 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'mycontainer'
  properties: {
    container: {
      image: 'myimage:latest'
    }
  }
}
`

	input := &ValidateAppDefinitionInput{
		BicepContent: bicepWithLatest,
		StrictMode:   true,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	warnings := filterWarnings(output.Issues)
	hasLatestWarning := false
	for _, w := range warnings {
		if w.Message == "using ':latest' tag for container images; consider using specific version tags" {
			hasLatestWarning = true
			break
		}
	}
	assert.True(t, hasLatestWarning)
}

func TestValidateAppDefinitionSkill_Execute_CrossReference(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	bicep := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}

resource api 'Applications.Core/containers@2023-10-01-preview' = {
  name: 'api'
  properties: {
    container: {
      image: 'api:v1'
    }
  }
}
`

	discoveryResult := &discovery.DiscoveryResult{
		Services: []discovery.Service{
			{Name: "api"},
			{Name: "worker"}, // Not in Bicep - should warn
		},
		ResourceTypes: []discovery.ResourceTypeMapping{
			{
				DependencyID: "redis_cache", // Not in Bicep - should warn
				ResourceType: discovery.ResourceType{Name: "Applications.Datastores/redisCaches"},
			},
		},
	}

	input := &ValidateAppDefinitionInput{
		BicepContent:    bicep,
		DiscoveryResult: discoveryResult,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	warnings := filterWarnings(output.Issues)
	assert.GreaterOrEqual(t, len(warnings), 2)

	hasWorkerWarning := false
	hasRedisWarning := false
	for _, w := range warnings {
		if w.Resource == "worker" {
			hasWorkerWarning = true
		}
		if w.Resource == "redis_cache" {
			hasRedisWarning = true
		}
	}
	assert.True(t, hasWorkerWarning, "should warn about missing worker service")
	assert.True(t, hasRedisWarning, "should warn about missing redis_cache resource")
}

func TestValidateAppDefinitionSkill_Execute_FromFile(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	// Create temp file
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "app.bicep")

	validBicep := `extension radius

param environment string

resource app 'Applications.Core/applications@2023-10-01-preview' = {
  name: 'myapp'
  properties: {
    environment: environment
  }
}
`

	err := os.WriteFile(tempFile, []byte(validBicep), 0644)
	require.NoError(t, err)

	input := &ValidateAppDefinitionInput{
		FilePath: tempFile,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	assert.True(t, output.Valid)
}

func TestValidateAppDefinitionSkill_Execute_NilInput(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	_, err := skill.Execute(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is required")
}

func TestValidateAppDefinitionSkill_Execute_NoContent(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	input := &ValidateAppDefinitionInput{}

	_, err := skill.Execute(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "either BicepContent or FilePath is required")
}

func TestValidateAppDefinitionSkill_Description(t *testing.T) {
	skill := NewValidateAppDefinitionSkill()

	desc := skill.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "Validate")
}

// Helper functions
func filterErrors(issues []ValidationIssue) []ValidationIssue {
	var errors []ValidationIssue
	for _, issue := range issues {
		if issue.Severity == "error" {
			errors = append(errors, issue)
		}
	}
	return errors
}

func filterWarnings(issues []ValidationIssue) []ValidationIssue {
	var warnings []ValidationIssue
	for _, issue := range issues {
		if issue.Severity == "warning" {
			warnings = append(warnings, issue)
		}
	}
	return warnings
}
