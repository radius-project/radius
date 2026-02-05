package practices

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBicepParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string
		wantTags []string
		wantErr  bool
	}{
		{
			name: "extracts tags from bicep",
			files: map[string]string{
				"main.bicep": `
resource storageAccount 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'projdevstorage001'
  location: location
  tags: {
    environment: 'dev'
    project: 'myproject'
    owner: 'team@example.com'
  }
  sku: {
    name: 'Standard_LRS'
  }
  kind: 'StorageV2'
}
`,
			},
			wantTags: []string{"environment", "project", "owner"},
		},
		{
			name: "extracts resource names",
			files: map[string]string{
				"storage.bicep": `
resource sa 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'proj-dev-storage-001'
}

resource kv 'Microsoft.KeyVault/vaults@2023-01-01' = {
  name: 'proj-dev-kv-001'
}
`,
			},
		},
		{
			name: "detects security settings",
			files: map[string]string{
				"db.bicep": `
resource psql 'Microsoft.DBforPostgreSQL/servers@2017-12-01' = {
  name: 'proj-dev-psql'
  properties: {
    minTlsVersion: 'TLS1_2'
    publicNetworkAccess: 'Disabled'
    sslEnforcement: 'Enabled'
  }
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

			parser := NewBicepParser(tmpDir)
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
					t.Errorf("expected tag %q not found in %v", tag, result.Tags)
				}
			}
		})
	}
}

func TestBicepParser_ExtractResourceNames(t *testing.T) {
	parser := &BicepParser{}

	content := `
resource storageAccount 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  name: 'proj-dev-storage-001'
}

resource keyVault 'Microsoft.KeyVault/vaults@2023-01-01' = {
  name: 'proj-dev-kv-001'
}
`

	names := parser.extractResourceNames(content)
	if len(names) < 2 {
		t.Errorf("expected at least 2 names, got %d: %v", len(names), names)
	}
}

func TestBicepParser_ExtractTags(t *testing.T) {
	parser := &BicepParser{}

	content := `
resource rg 'Microsoft.Resources/resourceGroups@2023-01-01' = {
  tags: {
    environment: 'dev'
    project: 'myproject'
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

func TestBicepParser_ExtractSecuritySettings(t *testing.T) {
	parser := &BicepParser{}

	tests := []struct {
		name    string
		content string
		check   func(*SecurityPractices) bool
	}{
		{
			name: "detects TLS",
			content: `
resource psql 'Microsoft.DBforPostgreSQL/servers@2017-12-01' = {
  properties: {
    minTlsVersion: 'TLS1_2'
  }
}
`,
			check: func(s *SecurityPractices) bool {
				return s.TLSRequired && s.MinTLSVersion == "TLS1_2"
			},
		},
		{
			name: "detects encryption",
			content: `
resource sa 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  properties: {
    encryption: {
      services: {
        blob: {
          enabled: true
        }
      }
    }
  }
}
`,
			check: func(s *SecurityPractices) bool {
				return s.EncryptionEnabled
			},
		},
		{
			name: "detects private endpoints",
			content: `
resource pe 'Microsoft.Network/privateEndpoints@2023-01-01' = {
  properties: {
    privateLinkServiceConnections: []
  }
}
`,
			check: func(s *SecurityPractices) bool {
				return s.PrivateNetworking
			},
		},
		{
			name: "detects disabled public access",
			content: `
resource sa 'Microsoft.Storage/storageAccounts@2023-01-01' = {
  properties: {
    publicNetworkAccess: 'Disabled'
  }
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

func TestBicepParser_NoFiles(t *testing.T) {
	tmpDir := t.TempDir()

	parser := NewBicepParser(tmpDir)
	result, err := parser.Parse()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != nil {
		t.Error("expected nil result for empty directory")
	}
}
