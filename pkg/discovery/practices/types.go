// Package practices provides team infrastructure practices detection and application.
package practices

import (
	"time"
)

// TeamPractices contains conventions extracted from existing IaC and configuration.
type TeamPractices struct {
	// NamingConvention is the detected naming pattern.
	NamingConvention *NamingPattern `json:"namingConvention,omitempty" yaml:"namingConvention,omitempty"`

	// Tags are common tags applied to resources.
	Tags map[string]string `json:"tags,omitempty" yaml:"tags,omitempty"`

	// RequiredTags are tags that must be present on all resources.
	RequiredTags []string `json:"requiredTags,omitempty" yaml:"requiredTags,omitempty"`

	// Environment is the detected environment context.
	Environment string `json:"environment,omitempty" yaml:"environment,omitempty"`

	// Region is the detected default region.
	Region string `json:"region,omitempty" yaml:"region,omitempty"`

	// Security contains security-related practices.
	Security SecurityPractices `json:"security,omitempty" yaml:"security,omitempty"`

	// Sizing contains sizing and tier practices.
	Sizing SizingPractices `json:"sizing,omitempty" yaml:"sizing,omitempty"`

	// Sources lists where practices were extracted from.
	Sources []PracticeSource `json:"sources,omitempty" yaml:"sources,omitempty"`

	// DetectedAt is when practices were detected.
	DetectedAt time.Time `json:"detectedAt,omitempty" yaml:"detectedAt,omitempty"`

	// Confidence is the overall confidence score (0.0-1.0).
	Confidence float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`
}

// NamingPattern describes a detected naming convention.
type NamingPattern struct {
	// Pattern is the naming pattern template (e.g., "{env}-{service}-{resource}").
	Pattern string `json:"pattern" yaml:"pattern"`

	// Examples are example names that match this pattern.
	Examples []string `json:"examples,omitempty" yaml:"examples,omitempty"`

	// Components are the individual pattern components.
	Components []PatternComponent `json:"components,omitempty" yaml:"components,omitempty"`

	// Confidence is the confidence score for this pattern (0.0-1.0).
	Confidence float64 `json:"confidence" yaml:"confidence"`
}

// PatternComponent describes a single component of a naming pattern.
type PatternComponent struct {
	// Name is the component name (e.g., "env", "service", "resource").
	Name string `json:"name" yaml:"name"`

	// Position is the position in the pattern (0-based).
	Position int `json:"position" yaml:"position"`

	// Values are detected values for this component.
	Values []string `json:"values,omitempty" yaml:"values,omitempty"`

	// Separator is the separator before this component.
	Separator string `json:"separator,omitempty" yaml:"separator,omitempty"`
}

// SecurityPractices contains security-related conventions.
type SecurityPractices struct {
	// EncryptionEnabled indicates if encryption is typically enabled.
	EncryptionEnabled bool `json:"encryptionEnabled" yaml:"encryptionEnabled"`

	// PrivateNetworking indicates if private networking is typically used.
	PrivateNetworking bool `json:"privateNetworking" yaml:"privateNetworking"`

	// TLSRequired indicates if TLS is required for connections.
	TLSRequired bool `json:"tlsRequired" yaml:"tlsRequired"`

	// MinTLSVersion is the minimum TLS version required.
	MinTLSVersion string `json:"minTlsVersion,omitempty" yaml:"minTlsVersion,omitempty"`

	// PublicAccessDisabled indicates if public access is typically disabled.
	PublicAccessDisabled bool `json:"publicAccessDisabled" yaml:"publicAccessDisabled"`
}

// SizingPractices contains sizing and tier conventions.
type SizingPractices struct {
	// DefaultTier is the default resource tier.
	DefaultTier string `json:"defaultTier,omitempty" yaml:"defaultTier,omitempty"`

	// EnvironmentTiers maps environments to tiers.
	EnvironmentTiers map[string]EnvironmentSizing `json:"environmentTiers,omitempty" yaml:"environmentTiers,omitempty"`
}

// EnvironmentSizing contains sizing practices for a specific environment.
type EnvironmentSizing struct {
	// Tier is the tier for this environment (e.g., "Basic", "Standard", "Premium").
	Tier string `json:"tier,omitempty" yaml:"tier,omitempty"`

	// HighAvailability indicates if HA is enabled for this environment.
	HighAvailability bool `json:"highAvailability" yaml:"highAvailability"`

	// GeoRedundant indicates if geo-redundancy is enabled.
	GeoRedundant bool `json:"geoRedundant" yaml:"geoRedundant"`

	// AutoShutdown is the auto-shutdown time (e.g., "19:00").
	AutoShutdown string `json:"autoShutdown,omitempty" yaml:"autoShutdown,omitempty"`
}

// PracticeSource indicates where a practice was extracted from.
type PracticeSource struct {
	// Type is the source type.
	Type PracticeSourceType `json:"type" yaml:"type"`

	// FilePath is the path to the source file.
	FilePath string `json:"filePath" yaml:"filePath"`

	// LineNumber is the specific line if applicable.
	LineNumber int `json:"lineNumber,omitempty" yaml:"lineNumber,omitempty"`

	// Confidence is the confidence for practices from this source.
	Confidence float64 `json:"confidence" yaml:"confidence"`

	// Resources is the list of resources defined in this file.
	Resources []IaCResource `json:"resources,omitempty" yaml:"resources,omitempty"`

	// Providers is the list of providers used in this file.
	Providers []string `json:"providers,omitempty" yaml:"providers,omitempty"`
}

// IaCResource represents a resource defined in an IaC file.
type IaCResource struct {
	Type string `json:"type" yaml:"type"` // e.g., "azurerm_resource_group"
	Name string `json:"name" yaml:"name"` // Logical name in the IaC
}

// PracticeSourceType categorizes practice sources.
type PracticeSourceType string

const (
	// SourceTerraform indicates practices from Terraform files.
	SourceTerraform PracticeSourceType = "terraform"

	// SourceBicep indicates practices from Bicep files.
	SourceBicep PracticeSourceType = "bicep"

	// SourceARM indicates practices from ARM templates.
	SourceARM PracticeSourceType = "arm"

	// SourceKubernetes indicates practices from Kubernetes manifests.
	SourceKubernetes PracticeSourceType = "kubernetes"

	// SourceEnvFile indicates practices from environment files.
	SourceEnvFile PracticeSourceType = "env-file"

	// SourceConfig indicates practices from config files.
	SourceConfig PracticeSourceType = "config"

	// SourceWiki indicates practices from wiki/documentation.
	SourceWiki PracticeSourceType = "wiki"
)

// PracticesConfig is the configuration file format for team practices.
type PracticesConfig struct {
	// Version is the config file version.
	Version string `json:"version" yaml:"version"`

	// Practices contains the team practices.
	Practices TeamPractices `json:"practices" yaml:"practices"`

	// Overrides are environment-specific overrides.
	Overrides map[string]TeamPractices `json:"overrides,omitempty" yaml:"overrides,omitempty"`
}

// Merge merges another TeamPractices into this one.
// Values from 'other' take precedence where present.
func (p *TeamPractices) Merge(other *TeamPractices) {
	if other == nil {
		return
	}

	if other.NamingConvention != nil {
		p.NamingConvention = other.NamingConvention
	}

	if other.Tags != nil {
		if p.Tags == nil {
			p.Tags = make(map[string]string)
		}
		for k, v := range other.Tags {
			p.Tags[k] = v
		}
	}

	if len(other.RequiredTags) > 0 {
		p.RequiredTags = append(p.RequiredTags, other.RequiredTags...)
	}

	if other.Environment != "" {
		p.Environment = other.Environment
	}

	if other.Region != "" {
		p.Region = other.Region
	}

	// Merge security practices
	if other.Security.EncryptionEnabled {
		p.Security.EncryptionEnabled = true
	}
	if other.Security.PrivateNetworking {
		p.Security.PrivateNetworking = true
	}
	if other.Security.TLSRequired {
		p.Security.TLSRequired = true
	}
	if other.Security.MinTLSVersion != "" {
		p.Security.MinTLSVersion = other.Security.MinTLSVersion
	}
	if other.Security.PublicAccessDisabled {
		p.Security.PublicAccessDisabled = true
	}

	// Merge sizing practices
	if other.Sizing.DefaultTier != "" {
		p.Sizing.DefaultTier = other.Sizing.DefaultTier
	}
	if other.Sizing.EnvironmentTiers != nil {
		if p.Sizing.EnvironmentTiers == nil {
			p.Sizing.EnvironmentTiers = make(map[string]EnvironmentSizing)
		}
		for k, v := range other.Sizing.EnvironmentTiers {
			p.Sizing.EnvironmentTiers[k] = v
		}
	}

	// Merge sources
	p.Sources = append(p.Sources, other.Sources...)
}

// GetTierForEnvironment returns the tier for a specific environment.
func (p *TeamPractices) GetTierForEnvironment(env string) string {
	if p.Sizing.EnvironmentTiers != nil {
		if sizing, ok := p.Sizing.EnvironmentTiers[env]; ok {
			return sizing.Tier
		}
	}
	return p.Sizing.DefaultTier
}

// ApplyNamingPattern applies the naming pattern with given values.
func (p *NamingPattern) ApplyNamingPattern(values map[string]string) string {
	if p == nil || p.Pattern == "" {
		return ""
	}

	result := p.Pattern
	for k, v := range values {
		placeholder := "{" + k + "}"
		// Simple string replacement
		for i := 0; i < 10; i++ { // Limit iterations to prevent infinite loop
			newResult := ""
			for j := 0; j < len(result); j++ {
				if j+len(placeholder) <= len(result) && result[j:j+len(placeholder)] == placeholder {
					newResult += v
					j += len(placeholder) - 1
				} else {
					newResult += string(result[j])
				}
			}
			if newResult == result {
				break
			}
			result = newResult
		}
	}
	return result
}
