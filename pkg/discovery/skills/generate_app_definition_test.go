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
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGenerateAppDefinitionSkill(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)
	require.NotNil(t, skill)
	assert.Equal(t, "generate_app_definition", skill.Name())
}

func TestGenerateAppDefinitionSkill_Execute_Basic(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	input := &GenerateAppDefinitionInput{
		DiscoveryResult: &discovery.DiscoveryResult{
			ProjectPath: "/test/myapp",
			Services: []discovery.Service{
				{
					Name:         "api",
					Language:     discovery.LanguageJavaScript,
					ExposedPorts: []int{3000},
				},
			},
			Dependencies:  []discovery.DetectedDependency{},
			ResourceTypes: []discovery.ResourceTypeMapping{},
		},
		ApplicationName: "testapp",
		IncludeComments: true,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Verify content generated
	assert.NotEmpty(t, output.BicepContent)
	assert.Contains(t, output.BicepContent, "extension radius")
	assert.Contains(t, output.BicepContent, "Applications.Core/applications")
	assert.Contains(t, output.BicepContent, "Applications.Core/containers")

	// Verify resource count
	assert.Equal(t, 2, output.ResourceCount) // 1 app + 1 container
}

func TestGenerateAppDefinitionSkill_Execute_NilInput(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	_, err = skill.Execute(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "input is required")
}

func TestGenerateAppDefinitionSkill_Execute_NilDiscoveryResult(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	input := &GenerateAppDefinitionInput{}

	_, err = skill.Execute(input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "discovery result is required")
}

func TestGenerateAppDefinitionSkill_Execute_Warnings(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	input := &GenerateAppDefinitionInput{
		DiscoveryResult: &discovery.DiscoveryResult{
			ProjectPath: "/test/myapp",
			Services: []discovery.Service{
				{
					Name:         "worker",
					Language:     discovery.LanguagePython,
					ExposedPorts: []int{}, // No ports - should trigger warning
				},
			},
			ResourceTypes: []discovery.ResourceTypeMapping{
				{
					DependencyID: "low-conf-db",
					ResourceType: discovery.ResourceType{Name: "Applications.Datastores/sqlDatabases"},
					Confidence:   0.5, // Low confidence - should trigger warning
				},
			},
		},
		ApplicationName: "testapp",
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)
	require.NotNil(t, output)

	// Should have warnings
	assert.Len(t, output.Warnings, 2)
	assert.Contains(t, output.Warnings[0], "no exposed ports")
	assert.Contains(t, output.Warnings[1], "low confidence")
}

func TestGenerateAppDefinitionSkill_Execute_WithResourceTypes(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	input := &GenerateAppDefinitionInput{
		DiscoveryResult: &discovery.DiscoveryResult{
			ProjectPath: "/test/myapp",
			Services: []discovery.Service{
				{
					Name:          "api",
					Language:      discovery.LanguageJavaScript,
					ExposedPorts:  []int{3000},
					DependencyIDs: []string{"postgres_db"},
				},
			},
			Dependencies: []discovery.DetectedDependency{
				{
					ID:      "postgres_db",
					Type:    discovery.DependencyPostgreSQL,
					Library: "pg",
				},
			},
			ResourceTypes: []discovery.ResourceTypeMapping{
				{
					DependencyID: "postgres_db",
					ResourceType: discovery.ResourceType{
						Name:       "Applications.Datastores/sqlDatabases",
						APIVersion: "2023-10-01-preview",
					},
					Confidence: 0.95,
				},
			},
		},
		ApplicationName: "testapp",
		IncludeComments: true,
	}

	output, err := skill.Execute(input)
	require.NoError(t, err)

	// Resource count: 1 app + 1 container + 1 db
	assert.Equal(t, 3, output.ResourceCount)

	// Verify infrastructure resource in output
	assert.Contains(t, output.BicepContent, "Applications.Datastores/sqlDatabases")
	assert.Contains(t, output.BicepContent, "postgres_db")
}

func TestGenerateAppDefinitionSkill_Description(t *testing.T) {
	skill, err := NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	desc := skill.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, strings.ToLower(desc), "generate")
	assert.Contains(t, strings.ToLower(desc), "app.bicep")
}
