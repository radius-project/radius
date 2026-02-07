// Package practices provides team infrastructure practices detection and application.
package practices

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// TerraformParser extracts team practices from Terraform files.
type TerraformParser struct {
	projectPath string
}

// NewTerraformParser creates a new TerraformParser.
func NewTerraformParser(projectPath string) *TerraformParser {
	return &TerraformParser{projectPath: projectPath}
}

// Parse extracts practices from all Terraform files in the project.
func (p *TerraformParser) Parse() (*TeamPractices, error) {
	practices := &TeamPractices{
		Tags: make(map[string]string),
	}

	// Find all .tf and .tfvars files
	tfFiles, err := p.findTerraformFiles()
	if err != nil {
		return nil, err
	}

	if len(tfFiles) == 0 {
		return nil, nil
	}

	var resourceNames []string
	var tagMaps []map[string]string
	var envConfigs []map[string]string

	for _, tfFile := range tfFiles {
		content, err := os.ReadFile(tfFile)
		if err != nil {
			continue
		}

		contentStr := string(content)

		// For .tfvars files, extract environment configurations
		if strings.HasSuffix(tfFile, ".tfvars") {
			envConfig := p.extractTfvarsConfig(contentStr)
			if len(envConfig) > 0 {
				envConfigs = append(envConfigs, envConfig)
			}
			// Don't add tfvars as a source with resources
			continue
		}

		// Extract resource names
		names := p.extractResourceNames(contentStr)
		resourceNames = append(resourceNames, names...)

		// Extract resources with types
		resources := p.extractResources(contentStr)

		// Extract providers
		providers := p.extractProviders(contentStr)

		// Extract tags
		tags := p.extractTags(contentStr)
		tagMaps = append(tagMaps, tags)

		// Extract security settings
		security := p.extractSecuritySettings(contentStr)
		mergeSecurity(&practices.Security, security)

		// Add source with detailed info
		practices.Sources = append(practices.Sources, PracticeSource{
			Type:       SourceTerraform,
			FilePath:   tfFile,
			Confidence: 0.8,
			Resources:  resources,
			Providers:  providers,
		})
	}

	// Detect naming pattern from resource names
	if len(resourceNames) > 0 {
		practices.NamingConvention = detectNamingPattern(resourceNames)
	}

	// Merge common tags from .tf files first
	practices.Tags = mergeTagMaps(tagMaps)
	practices.RequiredTags = detectRequiredTags(tagMaps)

	// Apply environment configurations from tfvars (these add to tags, not override)
	p.applyEnvConfigs(practices, envConfigs)

	// Detect environment tiers from variable defaults
	practices.Sizing.EnvironmentTiers = p.detectEnvironmentTiers(tfFiles)

	if len(practices.Sources) > 0 {
		practices.Confidence = 0.8
	}

	return practices, nil
}

func (p *TerraformParser) findTerraformFiles() ([]string, error) {
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
		if strings.HasSuffix(path, ".tf") || strings.HasSuffix(path, ".tfvars") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (p *TerraformParser) extractResourceNames(content string) []string {
	var names []string

	// Match resource "type" "name" { ... name = "value" ... }
	resourceRegex := regexp.MustCompile(`resource\s+"[^"]+"\s+"([^"]+)"`)
	matches := resourceRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			names = append(names, match[1])
		}
	}

	// Match name = "..." within resources
	nameRegex := regexp.MustCompile(`name\s*=\s*"([^"]+)"`)
	matches = nameRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			names = append(names, match[1])
		}
	}

	return names
}

// extractResources extracts resource type and name pairs from terraform content.
func (p *TerraformParser) extractResources(content string) []IaCResource {
	var resources []IaCResource

	// Match resource "type" "name"
	resourceRegex := regexp.MustCompile(`resource\s+"([^"]+)"\s+"([^"]+)"`)
	matches := resourceRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			resources = append(resources, IaCResource{
				Type: match[1],
				Name: match[2],
			})
		}
	}

	// Also match data sources
	dataRegex := regexp.MustCompile(`data\s+"([^"]+)"\s+"([^"]+)"`)
	matches = dataRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 2 {
			resources = append(resources, IaCResource{
				Type: "data." + match[1],
				Name: match[2],
			})
		}
	}

	return resources
}

// extractProviders extracts provider names from terraform content.
func (p *TerraformParser) extractProviders(content string) []string {
	providerSet := make(map[string]bool)

	// Match provider "name" blocks
	providerRegex := regexp.MustCompile(`provider\s+"([^"]+)"`)
	matches := providerRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			providerSet[match[1]] = true
		}
	}

	// Match required_providers blocks
	reqProviderRegex := regexp.MustCompile(`(\w+)\s*=\s*\{\s*source\s*=`)
	matches = reqProviderRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			providerSet[match[1]] = true
		}
	}

	// Infer from resource types (e.g., azurerm_resource_group -> azurerm)
	resourceRegex := regexp.MustCompile(`resource\s+"([^_]+)_[^"]+"`)
	matches = resourceRegex.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) > 1 {
			providerSet[match[1]] = true
		}
	}

	var providers []string
	for p := range providerSet {
		providers = append(providers, p)
	}
	return providers
}

func (p *TerraformParser) extractTags(content string) map[string]string {
	tags := make(map[string]string)

	// Match tags = { key = "value" } blocks
	tagsBlockRegex := regexp.MustCompile(`tags\s*=\s*\{([^}]+)\}`)
	matches := tagsBlockRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 1 {
			tagContent := match[1]
			// Extract individual tags
			tagRegex := regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)
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

func (p *TerraformParser) extractSecuritySettings(content string) *SecurityPractices {
	security := &SecurityPractices{}

	// Check for encryption settings
	if strings.Contains(content, "encryption") || strings.Contains(content, "encrypted") {
		security.EncryptionEnabled = true
	}

	// Check for private networking
	if strings.Contains(content, "private_endpoint") || strings.Contains(content, "private_subnet") {
		security.PrivateNetworking = true
	}

	// Check for TLS settings
	if strings.Contains(content, "min_tls_version") {
		tlsRegex := regexp.MustCompile(`min_tls_version\s*=\s*"([^"]+)"`)
		if match := tlsRegex.FindStringSubmatch(content); len(match) > 1 {
			security.MinTLSVersion = match[1]
			security.TLSRequired = true
		}
	}

	// Check for public access settings
	if strings.Contains(content, "public_network_access_enabled = false") ||
		strings.Contains(content, "public_access = \"Disabled\"") {
		security.PublicAccessDisabled = true
	}

	return security
}

func (p *TerraformParser) detectEnvironmentTiers(tfFiles []string) map[string]EnvironmentSizing {
	tiers := make(map[string]EnvironmentSizing)

	// Look for tfvars files for different environments
	varsPatterns := []string{"dev", "staging", "prod", "production", "test"}

	for _, pattern := range varsPatterns {
		for _, tfFile := range tfFiles {
			dir := filepath.Dir(tfFile)
			varsFile := filepath.Join(dir, pattern+".tfvars")
			if content, err := os.ReadFile(varsFile); err == nil {
				sizing := p.extractSizingFromVars(string(content))
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

func (p *TerraformParser) extractSizingFromVars(content string) EnvironmentSizing {
	sizing := EnvironmentSizing{}

	// Extract SKU/tier
	skuRegex := regexp.MustCompile(`sku\s*=\s*"([^"]+)"`)
	if match := skuRegex.FindStringSubmatch(content); len(match) > 1 {
		sizing.Tier = match[1]
	}

	// Check for HA settings
	if strings.Contains(content, "high_availability") || strings.Contains(content, "zone_redundant = true") {
		sizing.HighAvailability = true
	}

	// Check for geo-redundancy
	if strings.Contains(content, "geo_redundant = true") || strings.Contains(content, "GRS") {
		sizing.GeoRedundant = true
	}

	return sizing
}

// Helper functions

func mergeSecurity(target *SecurityPractices, source *SecurityPractices) {
	if source == nil {
		return
	}
	if source.EncryptionEnabled {
		target.EncryptionEnabled = true
	}
	if source.PrivateNetworking {
		target.PrivateNetworking = true
	}
	if source.TLSRequired {
		target.TLSRequired = true
	}
	if source.MinTLSVersion != "" {
		target.MinTLSVersion = source.MinTLSVersion
	}
	if source.PublicAccessDisabled {
		target.PublicAccessDisabled = true
	}
}

func mergeTagMaps(tagMaps []map[string]string) map[string]string {
	result := make(map[string]string)
	tagCounts := make(map[string]int)

	for _, tags := range tagMaps {
		for k, v := range tags {
			result[k] = v
			tagCounts[k]++
		}
	}

	// Only keep tags that appear in at least half the files
	threshold := len(tagMaps) / 2
	for k := range result {
		if tagCounts[k] < threshold {
			delete(result, k)
		}
	}

	return result
}

func detectRequiredTags(tagMaps []map[string]string) []string {
	if len(tagMaps) == 0 {
		return nil
	}

	tagCounts := make(map[string]int)
	for _, tags := range tagMaps {
		for k := range tags {
			tagCounts[k]++
		}
	}

	// Tags that appear in all (or almost all) files are considered required
	threshold := len(tagMaps) * 80 / 100 // 80%
	if threshold < 1 {
		threshold = 1
	}

	var required []string
	for k, count := range tagCounts {
		if count >= threshold {
			required = append(required, k)
		}
	}

	return required
}

func detectNamingPattern(names []string) *NamingPattern {
	if len(names) < 2 {
		return nil
	}

	// Common separators to check
	separators := []string{"-", "_", "."}

	var bestPattern string
	var bestConfidence float64

	for _, sep := range separators {
		pattern, confidence := analyzeNamingWithSeparator(names, sep)
		if confidence > bestConfidence {
			bestPattern = pattern
			bestConfidence = confidence
		}
	}

	if bestPattern == "" {
		return nil
	}

	return &NamingPattern{
		Pattern:    bestPattern,
		Examples:   names[:min(3, len(names))],
		Confidence: bestConfidence,
	}
}

func analyzeNamingWithSeparator(names []string, sep string) (string, float64) {
	// Count how many names use this separator
	separatedCount := 0
	var partCounts []int

	for _, name := range names {
		parts := strings.Split(name, sep)
		if len(parts) > 1 {
			separatedCount++
			partCounts = append(partCounts, len(parts))
		}
	}

	if separatedCount < len(names)/2 {
		return "", 0
	}

	// Find the most common part count
	countMap := make(map[int]int)
	for _, c := range partCounts {
		countMap[c]++
	}

	var mostCommonCount, maxOccurrences int
	for count, occurrences := range countMap {
		if occurrences > maxOccurrences {
			mostCommonCount = count
			maxOccurrences = occurrences
		}
	}

	// Build pattern based on most common structure
	var patternParts []string
	for i := 0; i < mostCommonCount; i++ {
		switch i {
		case 0:
			patternParts = append(patternParts, "{env}")
		case 1:
			patternParts = append(patternParts, "{app}")
		case 2:
			patternParts = append(patternParts, "{resource}")
		default:
			patternParts = append(patternParts, "{part"+string(rune('0'+i))+"}")
		}
	}

	pattern := strings.Join(patternParts, sep)
	confidence := float64(separatedCount) / float64(len(names))

	return pattern, confidence
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// extractTfvarsConfig extracts key-value pairs from tfvars content.
func (p *TerraformParser) extractTfvarsConfig(content string) map[string]string {
	config := make(map[string]string)

	// Match key = "value" or key = value patterns
	varRegex := regexp.MustCompile(`^\s*(\w+)\s*=\s*"?([^"\n]+)"?\s*$`)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Skip comments
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		matches := varRegex.FindStringSubmatch(line)
		if len(matches) > 2 {
			key := strings.TrimSpace(matches[1])
			value := strings.TrimSpace(matches[2])
			// Remove trailing quote if present
			value = strings.Trim(value, "\"")
			config[key] = value
		}
	}

	return config
}

// applyEnvConfigs applies environment configurations to practices.
func (p *TerraformParser) applyEnvConfigs(practices *TeamPractices, configs []map[string]string) {
	if len(configs) == 0 {
		return
	}

	// Collect unique values across all configs
	environments := make(map[string]bool)
	regions := make(map[string]bool)
	owners := make(map[string]bool)
	costCenters := make(map[string]bool)
	skus := make(map[string]map[string]bool) // env -> sku values

	for _, cfg := range configs {
		env := cfg["environment"]
		if env != "" {
			environments[env] = true
		}
		if loc, ok := cfg["location"]; ok && loc != "" {
			regions[loc] = true
		}
		if owner, ok := cfg["owner"]; ok && owner != "" {
			owners[owner] = true
		}
		if cc, ok := cfg["cost_center"]; ok && cc != "" {
			costCenters[cc] = true
		}
		// Collect SKU information per environment
		if env != "" {
			if skus[env] == nil {
				skus[env] = make(map[string]bool)
			}
			for key, value := range cfg {
				if strings.Contains(key, "sku") && value != "" {
					skus[env][value] = true
				}
			}
		}
	}

	// Set environment - show all environments found
	if len(environments) > 0 {
		var envList []string
		for env := range environments {
			envList = append(envList, env)
		}
		// Sort for consistent output (dev before prod)
		if len(envList) > 1 {
			// Simple sort: dev, prod, staging order
			sortedEnvs := sortEnvironments(envList)
			practices.Environment = strings.Join(sortedEnvs, ", ")
		} else {
			practices.Environment = envList[0]
		}
	}

	// Set region
	if len(regions) > 0 {
		var regionList []string
		for region := range regions {
			regionList = append(regionList, region)
		}
		practices.Region = strings.Join(regionList, ", ")
	}

	// Set tier/SKU info per environment
	if len(skus) > 0 {
		if practices.Sizing.EnvironmentTiers == nil {
			practices.Sizing.EnvironmentTiers = make(map[string]EnvironmentSizing)
		}
		var tierParts []string
		sortedEnvs := sortEnvironments(mapKeys(skus))
		for _, env := range sortedEnvs {
			envSkus := skus[env]
			var skuList []string
			for sku := range envSkus {
				skuList = append(skuList, sku)
			}
			if len(skuList) > 0 {
				tierStr := strings.Join(skuList, ", ")
				tierParts = append(tierParts, fmt.Sprintf("%s: %s", env, tierStr))
				practices.Sizing.EnvironmentTiers[env] = EnvironmentSizing{
					Tier: tierStr,
				}
			}
		}
		if len(tierParts) > 0 {
			practices.Sizing.DefaultTier = strings.Join(tierParts, " • ")
		}
	}

	// Add owner and cost center to tags
	if len(owners) > 0 {
		var ownerList []string
		for owner := range owners {
			ownerList = append(ownerList, owner)
		}
		practices.Tags["Owner"] = strings.Join(ownerList, ", ")
	}
	if len(costCenters) > 0 {
		var ccList []string
		for cc := range costCenters {
			ccList = append(ccList, cc)
		}
		practices.Tags["CostCenter"] = strings.Join(ccList, ", ")
	}
}

// mapKeys returns the keys of a map as a slice.
func mapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// sortEnvironments sorts environment names in a logical order.
func sortEnvironments(envs []string) []string {
	order := map[string]int{
		"dev": 1, "development": 1,
		"test": 2, "testing": 2,
		"staging": 3, "stage": 3,
		"prod": 4, "production": 4,
	}

	// Simple bubble sort for small lists
	sorted := make([]string, len(envs))
	copy(sorted, envs)
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			oi := order[strings.ToLower(sorted[i])]
			oj := order[strings.ToLower(sorted[j])]
			if oi == 0 {
				oi = 99
			}
			if oj == 0 {
				oj = 99
			}
			if oi > oj {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return sorted
}
