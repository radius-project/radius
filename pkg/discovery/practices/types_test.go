package practices

import (
	"testing"
)

func TestNamingPattern_ApplyNamingPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern NamingPattern
		values  map[string]string
		want    string
	}{
		{
			name: "simple pattern",
			pattern: NamingPattern{
				Pattern: "{project}-{resource}",
			},
			values: map[string]string{
				"project":  "myproj",
				"resource": "database",
			},
			want: "myproj-database",
		},
		{
			name: "full pattern",
			pattern: NamingPattern{
				Pattern: "{project}-{env}-{resource}-{instance}",
			},
			values: map[string]string{
				"project":  "app",
				"env":      "dev",
				"resource": "storage",
				"instance": "001",
			},
			want: "app-dev-storage-001",
		},
		{
			name: "empty pattern",
			pattern: NamingPattern{
				Pattern: "",
			},
			values: map[string]string{"resource": "unchanged"},
			want:   "",
		},
		{
			name: "partial values",
			pattern: NamingPattern{
				Pattern: "{env}-{resource}",
			},
			values: map[string]string{
				"env": "prod",
				// resource not provided
			},
			want: "prod-{resource}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.pattern.ApplyNamingPattern(tt.values)
			if got != tt.want {
				t.Errorf("ApplyNamingPattern() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTeamPractices_Merge(t *testing.T) {
	base := &TeamPractices{
		NamingConvention: &NamingPattern{
			Pattern: "{base}-{resource}",
		},
		Tags: map[string]string{
			"team":    "platform",
			"project": "base",
		},
		Security: SecurityPractices{
			TLSRequired: true,
		},
		Confidence: 0.8,
	}

	other := &TeamPractices{
		NamingConvention: &NamingPattern{
			Pattern: "{other}-{resource}",
		},
		Tags: map[string]string{
			"project": "other",
			"env":     "dev",
		},
		Security: SecurityPractices{
			EncryptionEnabled: true,
		},
		Confidence: 0.9,
	}

	base.Merge(other)

	// After merge, other's NamingConvention overwrites base (per Merge logic)
	if base.NamingConvention.Pattern != "{other}-{resource}" {
		t.Errorf("expected other pattern after merge, got %q", base.NamingConvention.Pattern)
	}

	// Should merge tags (other wins on conflicts per Merge logic)
	if base.Tags["team"] != "platform" {
		t.Error("expected team tag preserved")
	}
	if base.Tags["env"] != "dev" {
		t.Error("expected env tag from other")
	}
	if base.Tags["project"] != "other" {
		t.Error("expected project tag from other (conflict resolution)")
	}

	// Should merge security settings
	if !base.Security.TLSRequired {
		t.Error("expected TLSRequired preserved")
	}
	if !base.Security.EncryptionEnabled {
		t.Error("expected EncryptionEnabled from other")
	}
}

func TestTeamPractices_GetTierForEnvironment(t *testing.T) {
	practices := &TeamPractices{
		Sizing: SizingPractices{
			DefaultTier: "Standard",
			EnvironmentTiers: map[string]EnvironmentSizing{
				"dev":  {Tier: "Basic"},
				"prod": {Tier: "Premium"},
			},
		},
	}

	tests := []struct {
		env  string
		want string
	}{
		{"dev", "Basic"},
		{"prod", "Premium"},
		{"staging", "Standard"}, // Falls back to default
		{"", "Standard"},        // Empty uses default
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			got := practices.GetTierForEnvironment(tt.env)
			if got != tt.want {
				t.Errorf("GetTierForEnvironment(%q) = %q, want %q", tt.env, got, tt.want)
			}
		})
	}
}

func TestSourceTypeConstants(t *testing.T) {
	// Verify source type constants are defined
	sources := []PracticeSourceType{
		SourceTerraform,
		SourceBicep,
		SourceARM,
		SourceKubernetes,
		SourceEnvFile,
		SourceConfig,
		SourceWiki,
	}

	for _, s := range sources {
		if s == "" {
			t.Error("found empty source type constant")
		}
	}
}

func TestPracticeSource(t *testing.T) {
	source := PracticeSource{
		Type:       SourceTerraform,
		FilePath:   "/path/to/main.tf",
		Confidence: 0.85,
	}

	if source.Type != SourceTerraform {
		t.Errorf("expected SourceTerraform, got %v", source.Type)
	}
	if source.FilePath != "/path/to/main.tf" {
		t.Errorf("expected /path/to/main.tf, got %s", source.FilePath)
	}
	if source.Confidence != 0.85 {
		t.Errorf("expected 0.85, got %f", source.Confidence)
	}
}
