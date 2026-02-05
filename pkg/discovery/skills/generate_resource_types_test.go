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
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/resourcetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateResourceTypesSkill_Execute(t *testing.T) {
	// Create catalog
	catalog := resourcetypes.NewCatalog()
	catalog.Add(resourcetypes.ResourceTypeEntry{
		DependencyType:   discovery.DependencyPostgreSQL,
		ResourceTypeName: "Applications.Datastores/sqlDatabases",
		APIVersion:       "2023-10-01-preview",
	})
	catalog.Add(resourcetypes.ResourceTypeEntry{
		DependencyType:   discovery.DependencyRedis,
		ResourceTypeName: "Applications.Datastores/redisCaches",
		APIVersion:       "2023-10-01-preview",
	})

	skill := NewGenerateResourceTypesSkill(catalog)

	tests := []struct {
		name           string
		input          SkillInput
		wantSuccess    bool
		wantMappings   int
		wantWarnings   int
	}{
		{
			name: "map dependencies from context",
			input: SkillInput{
				ProjectPath: "/test",
				Context: &SkillContext{
					Dependencies: []interface{}{
						map[string]interface{}{
							"id":         "postgres-1",
							"type":       "postgresql",
							"name":       "pg",
							"confidence": 0.95,
						},
						map[string]interface{}{
							"id":         "redis-1",
							"type":       "redis",
							"name":       "ioredis",
							"confidence": 0.90,
						},
					},
				},
			},
			wantSuccess:  true,
			wantMappings: 2,
			wantWarnings: 0,
		},
		{
			name: "handle unknown dependency type",
			input: SkillInput{
				ProjectPath: "/test",
				Context: &SkillContext{
					Dependencies: []interface{}{
						map[string]interface{}{
							"id":   "unknown-1",
							"type": "unknown-db",
							"name": "unknown",
						},
					},
				},
			},
			wantSuccess:  true,
			wantMappings: 0,
			wantWarnings: 1,
		},
		{
			name: "empty dependencies",
			input: SkillInput{
				ProjectPath: "/test",
				Context:     &SkillContext{},
			},
			wantSuccess:  true,
			wantMappings: 0,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := skill.Execute(context.Background(), tt.input)
			require.NoError(t, err)

			assert.Equal(t, tt.wantSuccess, output.Success)
			assert.Len(t, output.Warnings, tt.wantWarnings)

			if result, ok := output.Result.(map[string]interface{}); ok {
				if mappings, ok := result["resourceTypes"].([]discovery.ResourceTypeMapping); ok {
					assert.Len(t, mappings, tt.wantMappings)
				}
			}
		})
	}
}

func TestGenerateResourceTypesSkill_Name(t *testing.T) {
	skill := NewGenerateResourceTypesSkill(nil)
	assert.Equal(t, "generate_resource_types", skill.Name())
}

func TestGenerateResourceTypesSkill_InputSchema(t *testing.T) {
	skill := NewGenerateResourceTypesSkill(nil)
	schema := skill.InputSchema()

	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]interface{})
	assert.Contains(t, props, "dependencies")
}
