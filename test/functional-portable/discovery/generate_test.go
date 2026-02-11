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

package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateWorkflow_EndToEnd tests the full generate workflow from discovery results to Bicep output.
func TestGenerateWorkflow_EndToEnd(t *testing.T) {
	// Create temp directory for output
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "app.bicep")

	// Create discovery result (simulating output from rad app discover)
	discoveryResult := &discovery.DiscoveryResult{
		ProjectPath:     "/test/sample-app",
		AnalyzerVersion: "1.0.0",
		Services: []discovery.Service{
			{
				Name:         "web-frontend",
				Language:     discovery.LanguageJavaScript,
				ExposedPorts: []int{3000},
				EntryPoint: discovery.EntryPoint{
					File: "src/index.js",
					Type: discovery.EntryPointMain,
				},
				DependencyIDs: []string{},
			},
			{
				Name:         "api-service",
				Language:     discovery.LanguageGo,
				ExposedPorts: []int{8080},
				EntryPoint: discovery.EntryPoint{
					File: "cmd/api/main.go",
					Type: discovery.EntryPointMain,
				},
				DependencyIDs: []string{"postgres_db", "redis_cache"},
			},
		},
		Dependencies: []discovery.DetectedDependency{
			{
				ID:      "postgres_db",
				Type:    discovery.DependencyPostgreSQL,
				Library: "github.com/lib/pq",
				Version: "v1.10.9",

				Confidence: 0.95,
				Evidence: []discovery.Evidence{
					{
						File:    "go.mod",
						Line:    15,
						Type:    "manifest",
						Snippet: "github.com/lib/pq v1.10.9",
					},
				},
			},
			{
				ID:      "redis_cache",
				Type:    discovery.DependencyRedis,
				Library: "github.com/redis/go-redis/v9",
				Version: "v9.0.5",

				Confidence: 0.90,
				Evidence: []discovery.Evidence{
					{
						File:    "go.mod",
						Line:    18,
						Type:    "manifest",
						Snippet: "github.com/redis/go-redis/v9 v9.0.5",
					},
				},
			},
		},
		ResourceTypes: []discovery.ResourceTypeMapping{
			{
				DependencyID: "postgres_db",
				ResourceType: discovery.ResourceType{
					Name:       "Radius.Data/postgreSqlDatabases",
					APIVersion: "2025-08-01-preview",
				},
				Confidence: 0.95,
			},
			{
				DependencyID: "redis_cache",
				ResourceType: discovery.ResourceType{
					Name:       "Radius.Data/redisCaches",
					APIVersion: "2025-08-01-preview",
				},
				Confidence: 0.90,
			},
		},
	}

	// Step 1: Generate application definition
	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	generateInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: discoveryResult,
		ApplicationName: "sample-app",
		Environment:     "default",
		OutputPath:      outputPath,
		IncludeComments: true,
		IncludeRecipes:  false,
	}

	generateOutput, err := generateSkill.Execute(generateInput)
	require.NoError(t, err)
	require.NotNil(t, generateOutput)

	// Verify file was created
	assert.FileExists(t, outputPath)
	assert.Equal(t, outputPath, generateOutput.OutputPath)

	// Verify resource count (1 app + 2 containers + 2 infra resources)
	assert.Equal(t, 5, generateOutput.ResourceCount)

	// Step 2: Validate generated Bicep
	validateSkill := skills.NewValidateAppDefinitionSkill()

	validateInput := &skills.ValidateAppDefinitionInput{
		FilePath:        outputPath,
		DiscoveryResult: discoveryResult,
		StrictMode:      true,
	}

	validateOutput, err := validateSkill.Execute(validateInput)
	require.NoError(t, err)
	require.NotNil(t, validateOutput)

	// Check validation passed (no error-level issues)
	errorCount := 0
	for _, issue := range validateOutput.Issues {
		if issue.Severity == "error" {
			t.Logf("Validation error: %s", issue.Message)
			errorCount++
		}
	}
	assert.Equal(t, 0, errorCount, "Generated Bicep should have no validation errors")

	// Step 3: Verify Bicep content
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	bicepContent := string(content)

	// Verify essential elements
	assert.Contains(t, bicepContent, "extension radius")
	assert.Contains(t, bicepContent, "param environment string")
	assert.Contains(t, bicepContent, "param applicationName string = 'sample-app'")
	assert.Contains(t, bicepContent, "Applications.Core/applications")

	// Verify containers
	assert.Contains(t, bicepContent, "web_frontend")
	assert.Contains(t, bicepContent, "api_service")
	assert.Contains(t, bicepContent, "Applications.Core/containers")

	// Verify infrastructure resources
	assert.Contains(t, bicepContent, "postgres_db")
	assert.Contains(t, bicepContent, "redis_cache")
	assert.Contains(t, bicepContent, "Radius.Data/postgreSqlDatabases")
	assert.Contains(t, bicepContent, "Radius.Data/redisCaches")

	// Verify connections
	assert.Contains(t, bicepContent, "connections:")
}

// TestGenerateWorkflow_FromDiscoveryJSON tests generating from a discovery.json file.
func TestGenerateWorkflow_FromDiscoveryJSON(t *testing.T) {
	tempDir := t.TempDir()
	discoveryPath := filepath.Join(tempDir, "discovery.json")
	outputPath := filepath.Join(tempDir, "app.bicep")

	// Create discovery.json
	discoveryResult := &discovery.DiscoveryResult{
		ProjectPath:     "/test/nodejs-app",
		AnalyzerVersion: "1.0.0",
		Services: []discovery.Service{
			{
				Name:          "api",
				Language:      discovery.LanguageJavaScript,
				ExposedPorts:  []int{3000},
				DependencyIDs: []string{"mongodb"},
			},
		},
		Dependencies: []discovery.DetectedDependency{
			{
				ID:         "mongodb",
				Type:       discovery.DependencyMongoDB,
				Library:    "mongodb",
				Version:    "5.0.0",
				Confidence: 0.92,
			},
		},
		ResourceTypes: []discovery.ResourceTypeMapping{
			{
				DependencyID: "mongodb",
				ResourceType: discovery.ResourceType{
					Name:       "Radius.Data/mongoDatabases",
					APIVersion: "2025-08-01-preview",
				},
				Confidence: 0.92,
			},
		},
	}

	data, err := json.MarshalIndent(discoveryResult, "", "  ")
	require.NoError(t, err)

	err = os.WriteFile(discoveryPath, data, 0644)
	require.NoError(t, err)

	// Load and parse discovery.json
	loadedData, err := os.ReadFile(discoveryPath)
	require.NoError(t, err)

	var loadedResult discovery.DiscoveryResult
	err = json.Unmarshal(loadedData, &loadedResult)
	require.NoError(t, err)

	// Generate Bicep
	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	generateInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: &loadedResult,
		ApplicationName: "nodejs-app",
		OutputPath:      outputPath,
		IncludeComments: true,
	}

	generateOutput, err := generateSkill.Execute(generateInput)
	require.NoError(t, err)

	// Verify
	assert.FileExists(t, outputPath)
	assert.Equal(t, 3, generateOutput.ResourceCount) // 1 app + 1 container + 1 mongo
	assert.Contains(t, generateOutput.BicepContent, "mongoDatabases")
}

// TestGenerateWorkflow_DryRun tests generating without writing files.
func TestGenerateWorkflow_DryRun(t *testing.T) {
	// Create discovery result
	discoveryResult := &discovery.DiscoveryResult{
		ProjectPath: "/test/dry-run-app",
		Services: []discovery.Service{
			{
				Name:         "service1",
				Language:     discovery.LanguagePython,
				ExposedPorts: []int{8000},
			},
		},
	}

	// Generate without output path (dry run)
	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	generateInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: discoveryResult,
		ApplicationName: "dry-run-app",
		OutputPath:      "", // No output path = dry run
		IncludeComments: true,
	}

	generateOutput, err := generateSkill.Execute(generateInput)
	require.NoError(t, err)

	// Content should be generated
	assert.NotEmpty(t, generateOutput.BicepContent)
	assert.Contains(t, generateOutput.BicepContent, "dry-run-app")

	// But no file should exist
	assert.Empty(t, generateOutput.OutputPath)
}

// TestGenerateWorkflow_MultipleServices tests generating with multiple interconnected services.
func TestGenerateWorkflow_MultipleServices(t *testing.T) {
	discoveryResult := &discovery.DiscoveryResult{
		ProjectPath: "/test/microservices",
		Services: []discovery.Service{
			{
				Name:          "gateway",
				Language:      discovery.LanguageGo,
				ExposedPorts:  []int{8080},
				DependencyIDs: []string{},
			},
			{
				Name:          "users",
				Language:      discovery.LanguageGo,
				ExposedPorts:  []int{8081},
				DependencyIDs: []string{"postgres_users"},
			},
			{
				Name:          "orders",
				Language:      discovery.LanguageJava,
				ExposedPorts:  []int{8082},
				DependencyIDs: []string{"postgres_orders", "rabbitmq"},
			},
			{
				Name:          "inventory",
				Language:      discovery.LanguagePython,
				ExposedPorts:  []int{8083},
				DependencyIDs: []string{"mongodb", "redis"},
			},
		},
		Dependencies: []discovery.DetectedDependency{
			{ID: "postgres_users", Type: discovery.DependencyPostgreSQL, Library: "pq"},
			{ID: "postgres_orders", Type: discovery.DependencyPostgreSQL, Library: "pq"},
			{ID: "rabbitmq", Type: discovery.DependencyRabbitMQ, Library: "amqp"},
			{ID: "mongodb", Type: discovery.DependencyMongoDB, Library: "pymongo"},
			{ID: "redis", Type: discovery.DependencyRedis, Library: "redis-py"},
		},
		ResourceTypes: []discovery.ResourceTypeMapping{
			{DependencyID: "postgres_users", ResourceType: discovery.ResourceType{Name: "Radius.Data/postgreSqlDatabases", APIVersion: "2025-08-01-preview"}, Confidence: 0.95},
			{DependencyID: "postgres_orders", ResourceType: discovery.ResourceType{Name: "Radius.Data/postgreSqlDatabases", APIVersion: "2025-08-01-preview"}, Confidence: 0.95},
			{DependencyID: "rabbitmq", ResourceType: discovery.ResourceType{Name: "Radius.Messaging/rabbitMQQueues", APIVersion: "2025-08-01-preview"}, Confidence: 0.90},
			{DependencyID: "mongodb", ResourceType: discovery.ResourceType{Name: "Radius.Data/mongoDatabases", APIVersion: "2025-08-01-preview"}, Confidence: 0.92},
			{DependencyID: "redis", ResourceType: discovery.ResourceType{Name: "Radius.Data/redisCaches", APIVersion: "2025-08-01-preview"}, Confidence: 0.88},
		},
	}

	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	generateInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: discoveryResult,
		ApplicationName: "microservices",
		IncludeComments: true,
	}

	generateOutput, err := generateSkill.Execute(generateInput)
	require.NoError(t, err)

	// Verify resource count: 1 app + 4 containers + 5 infra = 10
	assert.Equal(t, 10, generateOutput.ResourceCount)

	// Verify all services are in output
	assert.Contains(t, generateOutput.BicepContent, "gateway")
	assert.Contains(t, generateOutput.BicepContent, "users")
	assert.Contains(t, generateOutput.BicepContent, "orders")
	assert.Contains(t, generateOutput.BicepContent, "inventory")

	// Verify all infrastructure resources
	assert.Contains(t, generateOutput.BicepContent, "postgres_users")
	assert.Contains(t, generateOutput.BicepContent, "postgres_orders")
	assert.Contains(t, generateOutput.BicepContent, "rabbitmq")
	assert.Contains(t, generateOutput.BicepContent, "mongodb")
	assert.Contains(t, generateOutput.BicepContent, "redis")

	// Verify connections are set up
	assert.True(t, strings.Count(generateOutput.BicepContent, "connections:") >= 3)
}

// TestGenerateWorkflow_Warnings tests that appropriate warnings are generated.
func TestGenerateWorkflow_Warnings(t *testing.T) {
	discoveryResult := &discovery.DiscoveryResult{
		ProjectPath: "/test/warning-app",
		Services: []discovery.Service{
			{
				Name:         "background-worker",
				Language:     discovery.LanguagePython,
				ExposedPorts: []int{}, // No ports - should warn
			},
		},
		ResourceTypes: []discovery.ResourceTypeMapping{
			{
				DependencyID: "uncertain-db",
				ResourceType: discovery.ResourceType{Name: "Radius.Data/postgreSqlDatabases"},
				Confidence:   0.4, // Low confidence - should warn
			},
		},
	}

	generateSkill, err := skills.NewGenerateAppDefinitionSkill()
	require.NoError(t, err)

	generateInput := &skills.GenerateAppDefinitionInput{
		DiscoveryResult: discoveryResult,
		ApplicationName: "warning-app",
	}

	generateOutput, err := generateSkill.Execute(generateInput)
	require.NoError(t, err)

	// Should have at least 2 warnings
	assert.GreaterOrEqual(t, len(generateOutput.Warnings), 2)

	// Check for specific warnings
	hasPortWarning := false
	hasConfidenceWarning := false
	for _, warning := range generateOutput.Warnings {
		if strings.Contains(warning, "no exposed ports") {
			hasPortWarning = true
		}
		if strings.Contains(warning, "low confidence") {
			hasConfidenceWarning = true
		}
	}
	assert.True(t, hasPortWarning, "should warn about missing ports")
	assert.True(t, hasConfidenceWarning, "should warn about low confidence")
}
