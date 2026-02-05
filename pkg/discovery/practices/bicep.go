// Package practices provides team infrastructure practices detection and application.
package practices

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// BicepParser extracts team practices from Bicep files.
type BicepParser struct {
	projectPath string
}

// NewBicepParser creates a new BicepParser.
func NewBicepParser(projectPath string) *BicepParser {
	return &BicepParser{projectPath: projectPath}
}

// Parse extracts practices from all Bicep files in the project.
func (p *BicepParser) Parse() (*TeamPractices, error) {
	practices := &TeamPractices{
		Tags: make(map[string]string),
	}

	// Find all .bicep files
	bicepFiles, err := p.findBicepFiles()
	if err != nil {
		return nil, err
	}

	if len(bicepFiles) == 0 {
		return nil, nil
	}

	var resourceNames []string
	var tagMaps []map[string]string

	for _, bicepFile := range bicepFiles {
		content, err := os.ReadFile(bicepFile)
		if err != nil {
			continue
		}

		// Extract resource names
		names := p.extractResourceNames(string(content))
		resourceNames = append(resourceNames, names...)

		// Extract tags
		tags := p.extractTags(string(content))
		tagMaps = append(tagMaps, tags)

		// Extract security settings
		security := p.extractSecuritySettings(string(content))
		mergeSecurity(&practices.Security, security)

		// Extract parameters for sizing
		sizing := p.extractSizingFromParameters(string(content))
		if sizing.DefaultTier != "" {
			practices.Sizing.DefaultTier = sizing.DefaultTier
		}

		// Add source
		practices.Sources = append(practices.Sources, PracticeSource{
			Type:       SourceBicep,
			FilePath:   bicepFile,
			Confidence: 0.85,
		})
	}

	// Detect naming pattern from resource names
	if len(resourceNames) > 0 {
		practices.NamingConvention = detectNamingPattern(resourceNames)
	}

	// Merge common tags
	practices.Tags = mergeTagMaps(tagMaps)
	practices.RequiredTags = detectRequiredTags(tagMaps)

	// Detect environment-specific settings from parameter files
	practices.Sizing.EnvironmentTiers = p.detectEnvironmentTiersFromParamFiles(bicepFiles)

	if len(practices.Sources) > 0 {
		practices.Confidence = 0.85
	}

	return practices, nil
}

func (p *BicepParser) findBicepFiles() ([]string, error) {
	var files []string
	err := filepath.Walk(p.projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if info.IsDir() {
			// Skip hidden directories and common non-IaC directories
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "node_modules" || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".bicep") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (p *BicepParser) extractResourceNames(content string) []string {
	var names []string

	// Match resource symbolic names: resource <name> '...'
	resourceRegex := regexp.MustCompile(`resource\s+(\w+)\s+'[^']+'`)
	matches := resourceRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			names = append(names, match[1])
		}
	}

	// Match name property: name: '...'
	nameRegex := regexp.MustCompile(`name:\s*'([^']+)'`)
	matches = nameRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			names = append(names, match[1])
		}
	}

	// Match name with string interpolation
	nameInterpRegex := regexp.MustCompile(`name:\s*'([^$']*)\$\{[^}]+\}([^']*)'`)
	matches = nameInterpRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			// Extract the pattern without interpolation
			pattern := match[1] + "{var}" + match[2]
			names = append(names, pattern)
		}
	}

	return names
}

func (p *BicepParser) extractTags(content string) map[string]string {
	tags := make(map[string]string)

	// Match tags object: tags: { key: 'value' }
	tagsBlockRegex := regexp.MustCompile(`tags:\s*\{([^}]+)\}`)
	matches := tagsBlockRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			tagContent := match[1]
			// Extract individual tags
			tagRegex := regexp.MustCompile(`(\w+):\s*'([^']*)'`)
			tagMatches := tagRegex.FindAllStringSubmatch(tagContent, -1)
			for _, tm := range tagMatches {
				if len(tm) > 2 {
					tags[tm[1]] = tm[2]
				}
			}
		}
	}

	return tags
}

func (p *BicepParser) extractSecuritySettings(content string) *SecurityPractices {
	security := &SecurityPractices{}

	// Check for encryption settings
	if strings.Contains(content, "encryption") || strings.Contains(content, "customerManagedKey") {
		security.EncryptionEnabled = true
	}

	// Check for private endpoints
	if strings.Contains(content, "privateEndpoint") || strings.Contains(content, "privateLinkServiceConnection") {
		security.PrivateNetworking = true
	}

	// Check for TLS settings
	if strings.Contains(content, "minTlsVersion") {
		tlsRegex := regexp.MustCompile(`minTlsVersion:\s*'([^']+)'`)
		if match := tlsRegex.FindStringSubmatch(content); len(match) > 1 {
			security.MinTLSVersion = match[1]
			security.TLSRequired = true
		}
	}

	// Check for public access settings
	if strings.Contains(content, "publicNetworkAccess: 'Disabled'") ||
		strings.Contains(content, "allowPublicAccess: false") {
		security.PublicAccessDisabled = true
	}

	return security
}

func (p *BicepParser) extractSizingFromParameters(content string) SizingPractices {
	sizing := SizingPractices{}

	// Look for SKU parameters with defaults
	skuRegex := regexp.MustCompile(`@allowed\(\[([^\]]+)\]\)\s*param\s+\w*[sS]ku\w*\s+string\s*=\s*'([^']+)'`)
	if match := skuRegex.FindStringSubmatch(content); len(match) > 2 {
		sizing.DefaultTier = match[2]
	}

	// Simpler SKU parameter detection
	simpleSkuRegex := regexp.MustCompile(`param\s+\w*[sS]ku\w*\s+string\s*=\s*'([^']+)'`)
	if match := simpleSkuRegex.FindStringSubmatch(content); len(match) > 1 {
		if sizing.DefaultTier == "" {
			sizing.DefaultTier = match[1]
		}
	}

	return sizing
}

func (p *BicepParser) detectEnvironmentTiersFromParamFiles(bicepFiles []string) map[string]EnvironmentSizing {
	tiers := make(map[string]EnvironmentSizing)

	// Look for .bicepparam or .parameters.json files
	envPatterns := []string{"dev", "staging", "prod", "production", "test"}

	for _, bicepFile := range bicepFiles {
		dir := filepath.Dir(bicepFile)
		baseName := strings.TrimSuffix(filepath.Base(bicepFile), ".bicep")

		for _, pattern := range envPatterns {
			// Check for .bicepparam files
			paramFile := filepath.Join(dir, baseName+"."+pattern+".bicepparam")
			if content, err := os.ReadFile(paramFile); err == nil {
				sizing := p.extractSizingFromParamFile(string(content))
				envName := pattern
				if pattern == "production" {
					envName = "prod"
				}
				tiers[envName] = sizing
				continue
			}

			// Check for .parameters.json files
			jsonParamFile := filepath.Join(dir, baseName+"."+pattern+".parameters.json")
			if content, err := os.ReadFile(jsonParamFile); err == nil {
				sizing := p.extractSizingFromParamFile(string(content))
				envName := pattern
				if pattern == "production" {
					envName = "prod"
				}
				tiers[envName] = sizing
			}
		}
	}

	return tiers
}

func (p *BicepParser) extractSizingFromParamFile(content string) EnvironmentSizing {
	sizing := EnvironmentSizing{}

	// Extract SKU value
	skuRegex := regexp.MustCompile(`[sS]ku['"]*\s*[:=]\s*['"]?(\w+)['"]?`)
	if match := skuRegex.FindStringSubmatch(content); len(match) > 1 {
		sizing.Tier = match[1]
	}

	// Check for HA settings
	if strings.Contains(strings.ToLower(content), "highavailability") ||
		strings.Contains(content, "zoneRedundant: true") ||
		strings.Contains(content, "\"zoneRedundant\": true") {
		sizing.HighAvailability = true
	}

	// Check for geo-redundancy
	if strings.Contains(strings.ToLower(content), "georedu") ||
		strings.Contains(content, "GRS") {
		sizing.GeoRedundant = true
	}

	return sizing
}
