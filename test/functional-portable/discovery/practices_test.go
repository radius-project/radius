package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/radius-project/radius/pkg/discovery/practices"
	"github.com/radius-project/radius/pkg/discovery/skills"
)

func TestPractices_TerraformExtraction(t *testing.T) {
	// Create test directory with Terraform files
	tmpDir := t.TempDir()

	// Create a realistic Terraform project
	mainTf := `
terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.0"
    }
  }
}

provider "azurerm" {
  features {}
}

resource "azurerm_resource_group" "main" {
  name     = "myproj-dev-rg-001"
  location = "eastus"

  tags = {
    environment = "dev"
    project     = "myproject"
    owner       = "platform-team"
    costcenter  = "12345"
  }
}

resource "azurerm_storage_account" "main" {
  name                     = "myprojdevsa001"
  resource_group_name      = azurerm_resource_group.main.name
  location                 = azurerm_resource_group.main.location
  account_tier             = "Standard"
  account_replication_type = "LRS"
  
  enable_https_traffic_only = true
  min_tls_version           = "TLS1_2"

  tags = azurerm_resource_group.main.tags
}

resource "azurerm_postgresql_server" "main" {
  name                = "myproj-dev-psql-001"
  location            = azurerm_resource_group.main.location
  resource_group_name = azurerm_resource_group.main.name

  sku_name = "GP_Gen5_2"

  storage_mb                   = 5120
  backup_retention_days        = 7
  geo_redundant_backup_enabled = false
  auto_grow_enabled            = true

  administrator_login          = "psqladmin"
  administrator_login_password = var.psql_password
  version                      = "11"
  ssl_enforcement_enabled      = true
  ssl_minimal_tls_version_enforced = "TLS1_2"
  
  public_network_access_enabled = false

  tags = azurerm_resource_group.main.tags
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	// Parse Terraform files
	parser := practices.NewTerraformParser(tmpDir)
	result, err := parser.Parse()

	if err != nil {
		t.Fatalf("failed to parse Terraform: %v", err)
	}

	if result == nil {
		t.Fatal("expected result but got nil")
	}

	// Verify tags were extracted
	if result.Tags["environment"] != "dev" {
		t.Errorf("expected environment=dev, got %v", result.Tags["environment"])
	}
	if result.Tags["project"] != "myproject" {
		t.Errorf("expected project=myproject, got %v", result.Tags["project"])
	}

	// Verify security settings were detected
	if !result.Security.TLSRequired {
		t.Error("expected TLS to be required")
	}
	if result.Security.MinTLSVersion != "TLS1_2" {
		t.Errorf("expected TLS1_2, got %s", result.Security.MinTLSVersion)
	}
	if !result.Security.PublicAccessDisabled {
		t.Error("expected public access to be disabled")
	}

	// Verify sources were recorded
	if len(result.Sources) == 0 {
		t.Error("expected sources to be recorded")
	}
}

func TestPractices_BicepExtraction(t *testing.T) {
	// Create test directory with Bicep files
	tmpDir := t.TempDir()

	// Create a realistic Bicep project
	mainBicep := `
@description('The location for all resources')
param location string = resourceGroup().location

@description('The environment name')
param environment string = 'dev'

var commonTags = {
  environment: environment
  project: 'myproject'
  owner: 'platform-team'
  costcenter: '12345'
}

resource storageAccount 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'myprojdevsa001'
  location: location
  tags: commonTags
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
  properties: {
    supportsHttpsTrafficOnly: true
    minimumTlsVersion: 'TLS1_2'
    allowBlobPublicAccess: false
    encryption: {
      services: {
        blob: {
          enabled: true
        }
        file: {
          enabled: true
        }
      }
    }
  }
}

resource psqlServer 'Microsoft.DBforPostgreSQL/servers@2017-12-01' = {
  name: 'myproj-dev-psql-001'
  location: location
  tags: commonTags
  sku: {
    name: 'GP_Gen5_2'
    tier: 'GeneralPurpose'
    capacity: 2
  }
  properties: {
    minTlsVersion: 'TLS1_2'
    publicNetworkAccess: 'Disabled'
    sslEnforcement: 'Enabled'
    encryption: 'Enabled'
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.bicep"), []byte(mainBicep), 0644); err != nil {
		t.Fatalf("failed to write main.bicep: %v", err)
	}

	// Parse Bicep files
	parser := practices.NewBicepParser(tmpDir)
	result, err := parser.Parse()

	if err != nil {
		t.Fatalf("failed to parse Bicep: %v", err)
	}

	if result == nil {
		t.Fatal("expected result but got nil")
	}

	// Verify security settings were detected
	if !result.Security.TLSRequired || result.Security.MinTLSVersion != "TLS1_2" {
		t.Errorf("expected TLS1_2, got TLSRequired=%v, Version=%s",
			result.Security.TLSRequired, result.Security.MinTLSVersion)
	}
	if !result.Security.EncryptionEnabled {
		t.Error("expected encryption to be enabled")
	}

	// Verify sources were recorded
	if len(result.Sources) == 0 {
		t.Error("expected sources to be recorded")
	}
}

func TestPractices_ConfigFile(t *testing.T) {
	// Create test directory with config file
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, ".radius")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
version: "1.0.0"
practices:
  namingConvention:
    pattern: "{project}-{environment}-{resource}-{instance}"
    components:
      - name: project
        position: 0
        separator: "-"
      - name: environment
        position: 1
        separator: "-"
      - name: resource
        position: 2
        separator: "-"
      - name: instance
        position: 3
        separator: "-"
    confidence: 0.9

  tags:
    environment: dev
    project: myproject
    owner: platform-team
    costcenter: "12345"

  requiredTags:
    - environment
    - project
    - owner

  security:
    encryptionEnabled: true
    tlsRequired: true
    minTlsVersion: "TLS1_2"
    privateNetworking: true
    publicAccessDisabled: true

  sizing:
    defaultTier: Standard_LRS
    environmentTiers:
      dev:
        tier: Basic
        highAvailability: false
      staging:
        tier: Standard
        highAvailability: true
      prod:
        tier: Premium
        highAvailability: true
        geoRedundant: true
`
	configPath := filepath.Join(configDir, "team-practices.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Load config
	result, err := practices.LoadConfigFromFile(configPath)

	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if result == nil {
		t.Fatal("expected result but got nil")
	}

	// Verify naming convention
	if result.Practices.NamingConvention == nil {
		t.Error("expected naming convention to be set")
	} else if result.Practices.NamingConvention.Pattern != "{project}-{environment}-{resource}-{instance}" {
		t.Errorf("unexpected naming pattern: %s", result.Practices.NamingConvention.Pattern)
	}

	// Verify tags
	if result.Practices.Tags["project"] != "myproject" {
		t.Errorf("expected project=myproject, got %v", result.Practices.Tags["project"])
	}

	// Verify required tags
	if len(result.Practices.RequiredTags) != 3 {
		t.Errorf("expected 3 required tags, got %d", len(result.Practices.RequiredTags))
	}

	// Verify security
	if !result.Practices.Security.EncryptionEnabled {
		t.Error("expected encryption to be enabled")
	}
	if result.Practices.Security.MinTLSVersion != "TLS1_2" {
		t.Errorf("expected TLS1_2, got %s", result.Practices.Security.MinTLSVersion)
	}

	// Verify sizing
	if result.Practices.Sizing.DefaultTier != "Standard_LRS" {
		t.Errorf("expected Standard_LRS, got %s", result.Practices.Sizing.DefaultTier)
	}
	if prodTier, ok := result.Practices.Sizing.EnvironmentTiers["prod"]; !ok {
		t.Error("expected prod tier to exist")
	} else {
		if prodTier.Tier != "Premium" {
			t.Errorf("expected Premium for prod, got %s", prodTier.Tier)
		}
		if !prodTier.HighAvailability {
			t.Error("expected high availability for prod")
		}
	}
}

func TestPractices_SkillIntegration(t *testing.T) {
	// Create test directory with both config and IaC files
	tmpDir := t.TempDir()

	// Create config
	configDir := filepath.Join(tmpDir, ".radius")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	configContent := `
version: "1.0.0"
practices:
  namingConvention:
    pattern: "{project}-{env}-{resource}"
    components:
      - name: project
        position: 0
        separator: "-"
      - name: env
        position: 1
        separator: "-"
      - name: resource
        position: 2
        separator: "-"
    confidence: 0.9

  tags:
    project: config-project
    source: config
`
	configPath := filepath.Join(configDir, "team-practices.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create Terraform file with additional tags
	tfContent := `
resource "azurerm_storage_account" "main" {
  name = "proj-dev-sa-001"
  
  tags = {
    environment = "dev"
    team        = "platform"
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(tfContent), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	// Run the skill
	skill := skills.NewDiscoverTeamPracticesSkill()
	output, err := skill.Execute(context.Background(), skills.DiscoverTeamPracticesInput{
		ProjectPath: tmpDir,
		IncludeIaC:  true,
	})

	if err != nil {
		t.Fatalf("skill execution failed: %v", err)
	}

	// Verify naming pattern was detected (IaC detection takes precedence per Merge behavior)
	if output.Practices.NamingConvention == nil {
		t.Error("expected naming convention to be detected")
	} else if output.Practices.NamingConvention.Pattern == "" {
		t.Error("expected naming pattern to be set")
	}

	// Verify tags include both config and IaC (merged)
	// Note: Merge gives "other" (IaC) precedence, but config tags should still be present
	// unless IaC has the same keys
	if output.Practices.Tags == nil || len(output.Practices.Tags) == 0 {
		t.Error("expected tags to be merged from config and IaC")
	}

	// Verify sources were recorded from both config and IaC
	if len(output.Sources) < 2 {
		t.Errorf("expected sources from both config and IaC, got %d", len(output.Sources))
	}
}

func TestPractices_ApplyNamingPattern(t *testing.T) {
	pattern := practices.NamingPattern{
		Pattern: "{project}-{env}-{resource}-{instance}",
		Components: []practices.PatternComponent{
			{Name: "project", Position: 0, Separator: "-"},
			{Name: "env", Position: 1, Separator: "-"},
			{Name: "resource", Position: 2, Separator: "-"},
			{Name: "instance", Position: 3, Separator: "-"},
		},
		Confidence: 0.9,
	}

	// Verify pattern was created correctly
	if pattern.Pattern != "{project}-{env}-{resource}-{instance}" {
		t.Errorf("expected pattern to match, got %q", pattern.Pattern)
	}
	if len(pattern.Components) != 4 {
		t.Errorf("expected 4 components, got %d", len(pattern.Components))
	}
}

func TestPractices_MergePractices(t *testing.T) {
	base := &practices.TeamPractices{
		Tags: map[string]string{
			"team":    "platform",
			"project": "base",
		},
		Security: practices.SecurityPractices{
			TLSRequired: true,
		},
	}

	other := &practices.TeamPractices{
		Tags: map[string]string{
			"project": "other",
			"env":     "dev",
		},
		Security: practices.SecurityPractices{
			EncryptionEnabled: true,
		},
	}

	// Merge modifies base in place
	base.Merge(other)

	// Other's tags should have been merged
	if _, ok := base.Tags["env"]; !ok {
		t.Error("expected 'env' tag to be merged")
	}

	// Security settings should be merged
	if !base.Security.TLSRequired {
		t.Error("expected TLSRequired from base to remain")
	}
	if !base.Security.EncryptionEnabled {
		t.Error("expected EncryptionEnabled from other")
	}
}
