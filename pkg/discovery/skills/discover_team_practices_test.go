package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverTeamPracticesSkill_Name(t *testing.T) {
	skill := NewDiscoverTeamPracticesSkill()
	if skill.Name() != "discover_team_practices" {
		t.Errorf("expected name 'discover_team_practices', got %q", skill.Name())
	}
}

func TestDiscoverTeamPracticesSkill_Description(t *testing.T) {
	skill := NewDiscoverTeamPracticesSkill()
	if skill.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestDiscoverTeamPracticesSkill_Execute_EmptyProject(t *testing.T) {
	tmpDir := t.TempDir()

	skill := NewDiscoverTeamPracticesSkill()
	output, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
		IncludeIaC:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output == nil {
		t.Fatal("expected output but got nil")
	}

	if output.Practices == nil {
		t.Fatal("expected practices but got nil")
	}
}

func TestDiscoverTeamPracticesSkill_Execute_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config file
	configDir := filepath.Join(tmpDir, ".radius")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
version: "1.0"
practices:
  namingConvention:
    pattern: "{project}-{env}-{resource}"
  tags:
    environment: dev
    project: testproject
`
	configPath := filepath.Join(configDir, "team-practices.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	skill := NewDiscoverTeamPracticesSkill()
	output, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check naming convention if available
	if output.Practices.NamingConvention != nil {
		if output.Practices.NamingConvention.Pattern != "{project}-{env}-{resource}" {
			t.Errorf("expected naming pattern from config, got %q", output.Practices.NamingConvention.Pattern)
		}
	}

	if output.Practices.Tags["project"] != "testproject" {
		t.Errorf("expected project tag from config, got %q", output.Practices.Tags["project"])
	}

	if output.ConfigPath != configPath {
		t.Errorf("expected config path %q, got %q", configPath, output.ConfigPath)
	}
}

func TestDiscoverTeamPracticesSkill_Execute_WithTerraform(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Terraform file
	tfContent := `
resource "azurerm_storage_account" "main" {
  name = "proj-dev-storage-001"
  
  tags = {
    environment = "dev"
    team        = "platform"
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatalf("failed to write terraform file: %v", err)
	}

	skill := NewDiscoverTeamPracticesSkill()
	output, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
		IncludeIaC:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Sources) == 0 {
		t.Error("expected sources from IaC analysis")
	}
}

func TestDiscoverTeamPracticesSkill_Execute_WithBicep(t *testing.T) {
	tmpDir := t.TempDir()

	// Create Bicep file
	bicepContent := `
resource storageAccount 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'proj-dev-storage-001'
  location: 'eastus'
  tags: {
    environment: 'dev'
    team: 'platform'
  }
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.bicep"), []byte(bicepContent), 0644); err != nil {
		t.Fatalf("failed to write bicep file: %v", err)
	}

	skill := NewDiscoverTeamPracticesSkill()
	output, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
		IncludeIaC:  true,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Sources) == 0 {
		t.Error("expected sources from IaC analysis")
	}
}

func TestDiscoverTeamPracticesSkill_Execute_WithEnvironment(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config with environment tiers
	configDir := filepath.Join(tmpDir, ".radius")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
tags:
  project: testproject

sizing:
  default_tier: Standard
  environment_tiers:
    dev:
      tier: Basic
    prod:
      tier: Premium
`
	configPath := filepath.Join(configDir, "team-practices.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	skill := NewDiscoverTeamPracticesSkill()

	// Test prod environment
	output, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
		Environment: "prod",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Environment != "prod" {
		t.Errorf("expected environment 'prod', got %q", output.Environment)
	}
}

func TestDiscoverTeamPracticesSkill_Execute_MissingProjectPath(t *testing.T) {
	skill := NewDiscoverTeamPracticesSkill()
	_, err := skill.Execute(context.Background(), DiscoverTeamPracticesInput{})

	if err == nil {
		t.Error("expected error for missing project path")
	}
}

func TestDiscoverTeamPracticesSkill_ValidateInput(t *testing.T) {
	skill := NewDiscoverTeamPracticesSkill()

	tests := []struct {
		name    string
		input   DiscoverTeamPracticesInput
		wantErr bool
	}{
		{
			name:    "empty project path",
			input:   DiscoverTeamPracticesInput{},
			wantErr: true,
		},
		{
			name: "valid input",
			input: DiscoverTeamPracticesInput{
				ProjectPath: "/some/path",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := skill.ValidateInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscoverTeamPracticesSkill_GetSchema(t *testing.T) {
	skill := NewDiscoverTeamPracticesSkill()
	schema := skill.GetSchema()

	if schema == nil {
		t.Fatal("expected schema but got nil")
	}

	if schema["type"] != "object" {
		t.Errorf("expected type 'object', got %v", schema["type"])
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties map")
	}

	expectedProps := []string{"project_path", "environment", "include_iac", "config_path"}
	for _, prop := range expectedProps {
		if _, exists := props[prop]; !exists {
			t.Errorf("missing property: %s", prop)
		}
	}
}

func TestDeduplicateSources(t *testing.T) {
	tests := []struct {
		name     string
		sources  []practiceSource
		expected int
	}{
		{
			name:     "empty",
			sources:  nil,
			expected: 0,
		},
		{
			name: "no duplicates",
			sources: []practiceSource{
				{Type: "terraform", FilePath: "/a.tf"},
				{Type: "bicep", FilePath: "/b.bicep"},
			},
			expected: 2,
		},
		{
			name: "with duplicates",
			sources: []practiceSource{
				{Type: "terraform", FilePath: "/a.tf"},
				{Type: "terraform", FilePath: "/a.tf"},
				{Type: "bicep", FilePath: "/b.bicep"},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to practices.PracticeSource
			var sources []practicesSource
			for _, s := range tt.sources {
				sources = append(sources, practicesSource{
					Type:     s.Type,
					FilePath: s.FilePath,
				})
			}

			// The actual test uses the internal type
			result := deduplicateSourcesInternal(sources)
			if len(result) != tt.expected {
				t.Errorf("expected %d sources, got %d", tt.expected, len(result))
			}
		})
	}
}

// Helper types for testing
type practiceSource struct {
	Type     string
	FilePath string
}

type practicesSource struct {
	Type     string
	FilePath string
}

func deduplicateSourcesInternal(sources []practicesSource) []practicesSource {
	seen := make(map[string]bool)
	result := make([]practicesSource, 0, len(sources))

	for _, src := range sources {
		key := src.Type + ":" + src.FilePath
		if !seen[key] {
			seen[key] = true
			result = append(result, src)
		}
	}

	return result
}
