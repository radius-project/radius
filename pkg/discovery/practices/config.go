// Package practices provides team infrastructure practices detection and application.
package practices

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultConfigPath is the default path for team practices config.
const DefaultConfigPath = ".radius/team-practices.yaml"

// LoadConfig loads team practices configuration from the standard locations.
// It checks (in order): .radius/team-practices.yaml, ~/.radius/team-practices.yaml
func LoadConfig(projectPath string) (*PracticesConfig, error) {
	// Try project-local config first
	localPath := filepath.Join(projectPath, DefaultConfigPath)
	if cfg, err := LoadConfigFromFile(localPath); err == nil {
		return cfg, nil
	}

	// Try user home config
	homeDir, err := os.UserHomeDir()
	if err == nil {
		homePath := filepath.Join(homeDir, DefaultConfigPath)
		if cfg, err := LoadConfigFromFile(homePath); err == nil {
			return cfg, nil
		}
	}

	// No config found, return nil (not an error)
	return nil, nil
}

// LoadConfigFromFile loads configuration from a specific file.
func LoadConfigFromFile(path string) (*PracticesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg PracticesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Add config file as a source
	cfg.Practices.Sources = append(cfg.Practices.Sources, PracticeSource{
		Type:       SourceConfig,
		FilePath:   path,
		Confidence: 1.0, // Config file has highest confidence
	})

	return &cfg, nil
}

// SaveConfig saves practices configuration to a file.
func SaveConfig(cfg *PracticesConfig, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// GetPracticesForEnvironment returns practices with environment-specific overrides applied.
func (c *PracticesConfig) GetPracticesForEnvironment(env string) *TeamPractices {
	if c == nil {
		return nil
	}

	// Start with base practices
	result := c.Practices

	// Apply environment override if present
	if env != "" {
		if override, ok := c.Overrides[env]; ok {
			result.Merge(&override)
		}
	}

	return &result
}

// DefaultPracticesConfig returns a default practices configuration template.
func DefaultPracticesConfig() *PracticesConfig {
	return &PracticesConfig{
		Version: "1.0",
		Practices: TeamPractices{
			NamingConvention: &NamingPattern{
				Pattern: "{env}-{app}-{resource}",
				Examples: []string{
					"dev-myapp-db",
					"prod-myapp-cache",
				},
				Confidence: 0.5,
			},
			Tags: map[string]string{
				"managed-by": "radius",
			},
			RequiredTags: []string{
				"environment",
				"owner",
				"cost-center",
			},
			Security: SecurityPractices{
				EncryptionEnabled: true,
				TLSRequired:       true,
				MinTLSVersion:     "1.2",
			},
			Sizing: SizingPractices{
				DefaultTier: "Standard",
				EnvironmentTiers: map[string]EnvironmentSizing{
					"dev": {
						Tier:             "Basic",
						HighAvailability: false,
						AutoShutdown:     "19:00",
					},
					"staging": {
						Tier:             "Standard",
						HighAvailability: false,
					},
					"prod": {
						Tier:             "Premium",
						HighAvailability: true,
						GeoRedundant:     true,
					},
				},
			},
		},
	}
}

// GenerateConfigTemplate generates a YAML template for team practices.
func GenerateConfigTemplate() string {
	cfg := DefaultPracticesConfig()
	data, _ := yaml.Marshal(cfg)
	return string(data)
}
