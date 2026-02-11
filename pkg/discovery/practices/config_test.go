package practices

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "valid config",
			content: `
version: "1.0"
practices:
  tags:
    environment: dev
    project: myproject
    owner: team@example.com
  security:
    encryptionEnabled: true
    tlsRequired: true
    minTlsVersion: "TLS1_2"
    privateNetworking: true
  sizing:
    defaultTier: Standard
    environmentTiers:
      dev:
        tier: Basic
        highAvailability: false
      prod:
        tier: Premium
        highAvailability: true
        geoRedundant: true
`,
		},
		{
			name: "minimal config",
			content: `
version: "1.0"
practices:
  tags:
    environment: dev
`,
		},
		{
			name:    "invalid yaml",
			content: `invalid: [unclosed`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, ".radius", "team-practices.yaml")
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				t.Fatalf("failed to create dir: %v", err)
			}
			if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			result, err := LoadConfigFromFile(configPath)

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
		})
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	result, err := LoadConfigFromFile("/nonexistent/path/config.yaml")

	if err == nil {
		t.Error("expected error for nonexistent file")
	}

	if result != nil {
		t.Error("expected nil result for nonexistent file")
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".radius", "team-practices.yaml")

	cfg := &PracticesConfig{
		Version: "1.0",
		Practices: TeamPractices{
			NamingConvention: &NamingPattern{
				Pattern:    "{project}-{env}-{resource}",
				Confidence: 0.8,
			},
			Tags: map[string]string{
				"environment": "dev",
				"project":     "test",
			},
			Security: SecurityPractices{
				TLSRequired:   true,
				MinTLSVersion: "TLS1_2",
			},
		},
	}

	err := SaveConfig(cfg, configPath)
	if err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Load and verify
	loaded, err := LoadConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("failed to reload config: %v", err)
	}

	if loaded.Practices.NamingConvention == nil {
		t.Fatal("expected naming convention but got nil")
	}

	if loaded.Practices.NamingConvention.Pattern != cfg.Practices.NamingConvention.Pattern {
		t.Errorf("naming pattern mismatch: got %q, want %q",
			loaded.Practices.NamingConvention.Pattern, cfg.Practices.NamingConvention.Pattern)
	}
}

func TestDefaultPracticesConfig(t *testing.T) {
	config := DefaultPracticesConfig()

	if config == nil {
		t.Fatal("expected non-nil default config")
	}

	if config.Practices.NamingConvention == nil {
		t.Fatal("expected naming convention in default config")
	}

	if config.Practices.NamingConvention.Pattern == "" {
		t.Error("expected default naming pattern")
	}
}

func TestGenerateConfigTemplate(t *testing.T) {
	template := GenerateConfigTemplate()

	if template == "" {
		t.Error("expected non-empty template")
	}

	// Verify it contains expected sections
	expectedSections := []string{
		"version:",
		"practices:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(template, section) {
			t.Errorf("template missing section: %s", section)
		}
	}
}

func TestGetPracticesForEnvironment(t *testing.T) {
	cfg := &PracticesConfig{
		Version: "1.0",
		Practices: TeamPractices{
			Tags: map[string]string{
				"project": "myapp",
			},
			Sizing: SizingPractices{
				DefaultTier: "Standard",
			},
		},
		Overrides: map[string]TeamPractices{
			"dev": {
				Sizing: SizingPractices{
					DefaultTier: "Basic",
				},
			},
			"prod": {
				Sizing: SizingPractices{
					DefaultTier: "Premium",
				},
			},
		},
	}

	// Test dev environment
	devPractices := cfg.GetPracticesForEnvironment("dev")
	if devPractices.Sizing.DefaultTier != "Basic" {
		t.Errorf("expected dev tier Basic, got %s", devPractices.Sizing.DefaultTier)
	}

	// Test prod environment
	prodPractices := cfg.GetPracticesForEnvironment("prod")
	if prodPractices.Sizing.DefaultTier != "Premium" {
		t.Errorf("expected prod tier Premium, got %s", prodPractices.Sizing.DefaultTier)
	}

	// Test unknown environment (should use default)
	unknownPractices := cfg.GetPracticesForEnvironment("unknown")
	if unknownPractices.Sizing.DefaultTier != "Standard" {
		t.Errorf("expected default tier Standard, got %s", unknownPractices.Sizing.DefaultTier)
	}
}

func TestNilConfig_GetPracticesForEnvironment(t *testing.T) {
	var cfg *PracticesConfig
	result := cfg.GetPracticesForEnvironment("dev")
	if result != nil {
		t.Error("expected nil result for nil config")
	}
}
