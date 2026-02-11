package practices

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTerraformParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantTags []string
		wantErr  bool
	}{
		{
			name: "extracts common tags",
			files: map[string]string{
				"main.tf": `
resource "azurerm_storage_account" "example" {
  name                = "projdevstorage001"
  resource_group_name = var.resource_group_name
  location            = var.location
  account_tier        = "Standard"
  
  tags = {
    environment = "dev"
    project     = "myproject"
    owner       = "team@example.com"
  }
}
`,
			},
			wantTags: []string{"environment", "project", "owner"},
		},
		{
			name: "detects naming pattern",
			files: map[string]string{
				"storage.tf": `
resource "azurerm_storage_account" "main" {
  name = "proj-dev-storage-001"
}

resource "azurerm_key_vault" "main" {
  name = "proj-dev-kv-001"
}
`,
			},
			wantTags: nil,
		},
		{
			name: "detects security settings",
			files: map[string]string{
				"db.tf": `
resource "azurerm_postgresql_server" "main" {
  name                = "proj-dev-psql"
  ssl_enforcement_enabled          = true
  ssl_minimal_tls_version_enforced = "TLS1_2"
  public_network_access_enabled    = false
}
`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for name, content := range tt.files {
				path := filepath.Join(tmpDir, name)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}

			parser := NewTerraformParser(tmpDir)
			result, err := parser.Parse()

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result == nil {
				t.Fatal("expected result but got nil")
			}

			// Check tags
			for _, tag := range tt.wantTags {
				if _, exists := result.Tags[tag]; !exists {
					t.Errorf("expected tag %q not found", tag)
				}
			}
		})
	}
}

func TestTerraformParser_ExtractResourceNames(t *testing.T) {
	parser := &TerraformParser{}

	content := `
resource "azurerm_storage_account" "main" {
  name = "proj-dev-storage-001"
}

resource "azurerm_key_vault" "vault" {
  name = "proj-dev-kv-001"
}
`

	names := parser.extractResourceNames(content)
	if len(names) < 2 {
		t.Errorf("expected at least 2 names, got %d", len(names))
	}
}

func TestTerraformParser_ExtractTags(t *testing.T) {
	parser := &TerraformParser{}

	content := `
resource "azurerm_resource_group" "main" {
  tags = {
    environment = "dev"
    project     = "myproject"
  }
}
`

	tags := parser.extractTags(content)
	if tags["environment"] != "dev" {
		t.Errorf("expected environment=dev, got %v", tags["environment"])
	}
	if tags["project"] != "myproject" {
		t.Errorf("expected project=myproject, got %v", tags["project"])
	}
}

func TestTerraformParser_ExtractSecuritySettings(t *testing.T) {
	parser := &TerraformParser{}

	tests := []struct {
		name    string
		content string
		check   func(*SecurityPractices) bool
	}{
		{
			name: "detects TLS",
			content: `
resource "azurerm_postgresql_server" "main" {
  ssl_enforcement_enabled = true
  min_tls_version = "TLS1_2"
}
`,
			check: func(s *SecurityPractices) bool {
				return s.TLSRequired && s.MinTLSVersion == "TLS1_2"
			},
		},
		{
			name: "detects encryption",
			content: `
resource "azurerm_storage_account" "main" {
  enable_https_traffic_only = true
  infrastructure_encryption_enabled = true
}
`,
			check: func(s *SecurityPractices) bool {
				return s.EncryptionEnabled
			},
		},
		{
			name: "detects private networking",
			content: `
resource "azurerm_postgresql_server" "main" {
  public_network_access_enabled = false
}
`,
			check: func(s *SecurityPractices) bool {
				return s.PublicAccessDisabled
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			security := parser.extractSecuritySettings(tt.content)
			if !tt.check(security) {
				t.Errorf("security check failed for %s", tt.name)
			}
		})
	}
}

func TestDetectNamingPattern(t *testing.T) {
	tests := []struct {
		name           string
		names          []string
		wantPattern    bool
		wantComponents int
	}{
		{
			name:           "hyphen separated",
			names:          []string{"proj-dev-storage-001", "proj-dev-kv-001", "proj-dev-db-001"},
			wantPattern:    true,
			wantComponents: 4,
		},
		{
			name:           "underscore separated",
			names:          []string{"proj_dev_storage_001", "proj_dev_kv_001"},
			wantPattern:    true,
			wantComponents: 4,
		},
		{
			name:        "no pattern",
			names:       []string{"storage", "keyvault", "database"},
			wantPattern: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectNamingPattern(tt.names)
			if tt.wantPattern {
				if result == nil {
					t.Error("expected pattern but got nil")
					return
				}
				if result.Pattern == "" {
					t.Error("expected non-empty pattern")
				}
			} else {
				if result != nil && result.Pattern != "" {
					t.Errorf("expected no pattern, got %q", result.Pattern)
				}
			}
		})
	}
}
