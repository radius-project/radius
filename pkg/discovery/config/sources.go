// Package config provides configuration loading for the discovery feature.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// RecipeSourceConfig contains configuration for a recipe source.
type RecipeSourceConfig struct {
	// Name is the source name.
	Name string `yaml:"name" json:"name"`

	// Type is the source type (avm, terraform, git, local).
	Type string `yaml:"type" json:"type"`

	// URL is the source URL or path.
	URL string `yaml:"url" json:"url"`

	// Priority determines source preference (lower = higher priority).
	Priority int `yaml:"priority" json:"priority"`

	// Auth contains authentication configuration.
	Auth *AuthConfig `yaml:"auth,omitempty" json:"auth,omitempty"`

	// Options are source-specific options.
	Options map[string]string `yaml:"options,omitempty" json:"options,omitempty"`

	// Profiles maps environment names to profile-specific overrides.
	Profiles map[string]ProfileOverride `yaml:"profiles,omitempty" json:"profiles,omitempty"`
}

// AuthConfig contains authentication configuration for a recipe source.
type AuthConfig struct {
	// Type is the auth type: token, basic, credential-helper, env.
	Type string `yaml:"type" json:"type"`

	// Token is an access token (or env var name if type=env).
	Token string `yaml:"token,omitempty" json:"token,omitempty"`

	// TokenEnvVar is the environment variable containing the token.
	TokenEnvVar string `yaml:"tokenEnvVar,omitempty" json:"tokenEnvVar,omitempty"`

	// Username for basic auth.
	Username string `yaml:"username,omitempty" json:"username,omitempty"`

	// Password for basic auth (or env var name if type=env).
	Password string `yaml:"password,omitempty" json:"password,omitempty"`

	// PasswordEnvVar is the environment variable containing the password.
	PasswordEnvVar string `yaml:"passwordEnvVar,omitempty" json:"passwordEnvVar,omitempty"`

	// CredentialHelper is the credential helper command.
	CredentialHelper string `yaml:"credentialHelper,omitempty" json:"credentialHelper,omitempty"`
}

// ProfileOverride contains environment-specific configuration overrides.
type ProfileOverride struct {
	// URL overrides the source URL for this profile.
	URL string `yaml:"url,omitempty" json:"url,omitempty"`

	// Options are profile-specific options.
	Options map[string]string `yaml:"options,omitempty" json:"options,omitempty"`

	// Enabled indicates if this source is enabled for this profile.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
}

// SourcesConfig is the top-level configuration for recipe sources.
type SourcesConfig struct {
	// Version is the config file version.
	Version string `yaml:"version" json:"version"`

	// DefaultProfile is the default profile to use.
	DefaultProfile string `yaml:"defaultProfile,omitempty" json:"defaultProfile,omitempty"`

	// Sources is the list of recipe sources.
	Sources []RecipeSourceConfig `yaml:"sources" json:"sources"`
}

// LoadSourcesConfig loads recipe source configuration from the standard locations.
// It checks (in order): .rad/config.yaml, ~/.rad/config.yaml
func LoadSourcesConfig() (*SourcesConfig, error) {
	// Try local config first
	localPath := filepath.Join(".rad", "config.yaml")
	if cfg, err := loadConfigFile(localPath); err == nil {
		return cfg, nil
	}

	// Try user home config
	homeDir, err := os.UserHomeDir()
	if err == nil {
		homePath := filepath.Join(homeDir, ".rad", "config.yaml")
		if cfg, err := loadConfigFile(homePath); err == nil {
			return cfg, nil
		}
	}

	// Return default empty config
	return &SourcesConfig{
		Version: "1.0",
		Sources: []RecipeSourceConfig{},
	}, nil
}

// LoadSourcesConfigFromFile loads configuration from a specific file.
func LoadSourcesConfigFromFile(path string) (*SourcesConfig, error) {
	return loadConfigFile(path)
}

func loadConfigFile(path string) (*SourcesConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var cfg SourcesConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	return &cfg, nil
}

// GetSourcesForProfile returns sources configured for the given profile.
// If profile is empty, uses the default profile or no profile filtering.
func (c *SourcesConfig) GetSourcesForProfile(profile string) []RecipeSourceConfig {
	if profile == "" {
		profile = c.DefaultProfile
	}

	var result []RecipeSourceConfig
	for _, src := range c.Sources {
		// Apply profile overrides if present
		configured := src
		if profile != "" {
			if override, ok := src.Profiles[profile]; ok {
				// Check if disabled for this profile
				if override.Enabled != nil && !*override.Enabled {
					continue
				}
				// Apply URL override
				if override.URL != "" {
					configured.URL = override.URL
				}
				// Merge options
				if override.Options != nil {
					if configured.Options == nil {
						configured.Options = make(map[string]string)
					}
					for k, v := range override.Options {
						configured.Options[k] = v
					}
				}
			}
		}
		result = append(result, configured)
	}

	return result
}

// ResolveAuth resolves authentication credentials from the configuration.
// It handles environment variable lookups and credential helpers.
func (a *AuthConfig) ResolveAuth() (*ResolvedAuth, error) {
	if a == nil {
		return nil, nil
	}

	resolved := &ResolvedAuth{}

	switch a.Type {
	case "token":
		resolved.Token = a.Token
	case "env":
		// Resolve token from environment variable
		if a.TokenEnvVar != "" {
			resolved.Token = os.Getenv(a.TokenEnvVar)
			if resolved.Token == "" {
				return nil, fmt.Errorf("environment variable %s is not set", a.TokenEnvVar)
			}
		}
		// Resolve password from environment variable
		if a.PasswordEnvVar != "" {
			resolved.Password = os.Getenv(a.PasswordEnvVar)
			resolved.Username = a.Username
		}
	case "basic":
		resolved.Username = a.Username
		resolved.Password = a.Password
	case "credential-helper":
		// Execute credential helper and parse output
		token, err := executeCredentialHelper(a.CredentialHelper)
		if err != nil {
			return nil, fmt.Errorf("credential helper failed: %w", err)
		}
		resolved.Token = token
	default:
		if a.Type != "" {
			return nil, fmt.Errorf("unknown auth type: %s", a.Type)
		}
	}

	return resolved, nil
}

// ResolvedAuth contains resolved authentication credentials.
type ResolvedAuth struct {
	// Token is the resolved access token.
	Token string

	// Username is the resolved username.
	Username string

	// Password is the resolved password.
	Password string
}

// HasCredentials returns true if any credentials are present.
func (r *ResolvedAuth) HasCredentials() bool {
	if r == nil {
		return false
	}
	return r.Token != "" || (r.Username != "" && r.Password != "")
}

// executeCredentialHelper runs a credential helper command and returns the token.
func executeCredentialHelper(helper string) (string, error) {
	if helper == "" {
		return "", fmt.Errorf("credential helper command is empty")
	}

	// Split command into parts
	parts := strings.Fields(helper)
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid credential helper command")
	}

	// Execute the command
	// Note: In production, use exec.Command properly
	// For now, we support simple environment variable expansion
	if strings.HasPrefix(parts[0], "$") {
		envVar := strings.TrimPrefix(parts[0], "$")
		return os.Getenv(envVar), nil
	}

	return "", fmt.Errorf("credential helper execution not fully implemented")
}

// SaveSourcesConfig saves the configuration to a file.
func SaveSourcesConfig(cfg *SourcesConfig, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// AddSource adds a new source to the configuration.
func (c *SourcesConfig) AddSource(source RecipeSourceConfig) error {
	// Check for duplicate names
	for _, existing := range c.Sources {
		if existing.Name == source.Name {
			return fmt.Errorf("source %q already exists", source.Name)
		}
	}

	c.Sources = append(c.Sources, source)
	return nil
}

// RemoveSource removes a source by name.
func (c *SourcesConfig) RemoveSource(name string) error {
	for i, src := range c.Sources {
		if src.Name == name {
			c.Sources = append(c.Sources[:i], c.Sources[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("source %q not found", name)
}
