// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// LocalTerraformSource discovers recipes from local Terraform modules in a project.
type LocalTerraformSource struct {
	name        string
	projectPath string
}

// NewLocalTerraformSource creates a new local Terraform recipe source.
func NewLocalTerraformSource(config SourceConfig) (*LocalTerraformSource, error) {
	projectPath := config.URL
	if projectPath == "" {
		// Use current directory as default
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	return &LocalTerraformSource{
		name:        config.Name,
		projectPath: projectPath,
	}, nil
}

// Name returns the source name.
func (s *LocalTerraformSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *LocalTerraformSource) Type() string {
	return "local-terraform"
}

// Search searches for local TF modules matching the resource type.
func (s *LocalTerraformSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	allRecipes, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	// Filter recipes that match the resource type
	var matched []Recipe
	for _, recipe := range allRecipes {
		if recipe.ResourceType == resourceType || s.resourceTypeMatches(recipe, resourceType) {
			matched = append(matched, recipe)
		}
	}

	return matched, nil
}

// List lists all available local TF module recipes.
func (s *LocalTerraformSource) List(ctx context.Context) ([]Recipe, error) {
	var recipes []Recipe

	// Common directories where TF modules are stored
	tfDirs := []string{
		"infra",
		"infrastructure",
		"terraform",
		"tf",
		"deploy",
		"iac",
	}

	for _, dir := range tfDirs {
		dirPath := filepath.Join(s.projectPath, dir)
		if _, err := os.Stat(dirPath); err == nil {
			discovered, err := s.discoverFromDirectory(dirPath)
			if err == nil {
				recipes = append(recipes, discovered...)
			}
		}
	}

	// Also check root level for TF files
	rootRecipes, err := s.discoverFromDirectory(s.projectPath)
	if err == nil {
		recipes = append(recipes, rootRecipes...)
	}

	return recipes, nil
}

// discoverFromDirectory discovers TF module recipes from a directory.
func (s *LocalTerraformSource) discoverFromDirectory(dirPath string) ([]Recipe, error) {
	var recipes []Recipe

	// Check if this directory contains TF files
	tfFiles, err := filepath.Glob(filepath.Join(dirPath, "*.tf"))
	if err != nil || len(tfFiles) == 0 {
		return nil, nil
	}

	// Analyze the TF files to determine what resources are provisioned
	resources, err := s.analyzeTerraformResources(dirPath)
	if err != nil {
		return nil, err
	}

	// Create recipes for each detected resource type
	for resourceType, resourceInfo := range resources {
		relativePath, _ := filepath.Rel(s.projectPath, dirPath)
		if relativePath == "" || relativePath == "." {
			relativePath = "."
		}

		recipe := Recipe{
			Name:         resourceInfo.Name,
			Description:  resourceInfo.Description,
			ResourceType: s.mapToRadiusResourceType(resourceType),
			Source:       s.name,
			SourceType:   "local-terraform",
			Version:      "local",
			TemplatePath: relativePath,
			Parameters:   resourceInfo.Parameters,
			Tags:         []string{"terraform", "local", resourceType},
		}
		recipes = append(recipes, recipe)
	}

	return recipes, nil
}

// TerraformResourceInfo holds information about a discovered TF resource.
type TerraformResourceInfo struct {
	Name        string
	Description string
	Type        string
	Parameters  []RecipeParameter
}

// analyzeTerraformResources analyzes TF files to find resource types.
func (s *LocalTerraformSource) analyzeTerraformResources(dirPath string) (map[string]TerraformResourceInfo, error) {
	resources := make(map[string]TerraformResourceInfo)

	files, err := filepath.Glob(filepath.Join(dirPath, "*.tf"))
	if err != nil {
		return nil, err
	}

	// Patterns for different resource types
	resourcePatterns := map[string]*regexp.Regexp{
		"mysql":     regexp.MustCompile(`resource\s+"azurerm_mysql_flexible_server"`),
		"redis":     regexp.MustCompile(`resource\s+"azurerm_redis_cache"`),
		"postgres":  regexp.MustCompile(`resource\s+"azurerm_postgresql_flexible_server"`),
		"mongodb":   regexp.MustCompile(`resource\s+"azurerm_cosmosdb_mongo_database"`),
		"sql":       regexp.MustCompile(`resource\s+"azurerm_mssql_server"`),
		"storage":   regexp.MustCompile(`resource\s+"azurerm_storage_account"`),
		"keyvault":  regexp.MustCompile(`resource\s+"azurerm_key_vault"`),
		"container": regexp.MustCompile(`resource\s+"azurerm_container_registry"`),
	}

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		contentStr := string(content)

		for resourceType, pattern := range resourcePatterns {
			if pattern.MatchString(contentStr) {
				if _, exists := resources[resourceType]; !exists {
					resources[resourceType] = TerraformResourceInfo{
						Name:        s.generateRecipeName(resourceType, dirPath),
						Description: s.generateDescription(resourceType, dirPath),
						Type:        resourceType,
						Parameters:  s.extractParameters(dirPath),
					}
				}
			}
		}
	}

	return resources, nil
}

// mapToRadiusResourceType maps TF resource types to Radius types.
func (s *LocalTerraformSource) mapToRadiusResourceType(tfType string) string {
	mappings := map[string]string{
		"mysql":     "Radius.Data/mySqlDatabases",
		"redis":     "Radius.Data/redisCaches",
		"postgres":  "Radius.Data/postgreSqlDatabases",
		"mongodb":   "Radius.Data/mongoDatabases",
		"sql":       "Radius.Data/sqlDatabases",
		"storage":   "Radius.Storage/blobContainers",
		"keyvault":  "Radius.Security/secrets",
		"container": "Radius.Compute/containers",
	}

	if radiusType, ok := mappings[tfType]; ok {
		return radiusType
	}
	return tfType
}

// resourceTypeMatches checks if a recipe matches a resource type.
func (s *LocalTerraformSource) resourceTypeMatches(recipe Recipe, resourceType string) bool {
	// Check if any tags match the resource type
	resourceLower := strings.ToLower(resourceType)
	for _, tag := range recipe.Tags {
		if strings.Contains(resourceLower, strings.ToLower(tag)) {
			return true
		}
	}
	return false
}

// generateRecipeName generates a recipe name from the resource type and directory.
func (s *LocalTerraformSource) generateRecipeName(resourceType, dirPath string) string {
	dirName := filepath.Base(dirPath)
	if dirName == "." || dirName == s.projectPath {
		dirName = "default"
	}
	return "local-" + resourceType + "-" + dirName
}

// generateDescription generates a description for the recipe.
func (s *LocalTerraformSource) generateDescription(resourceType, dirPath string) string {
	relativePath, _ := filepath.Rel(s.projectPath, dirPath)
	return "Terraform recipe for " + resourceType + " from local module: " + relativePath
}

// extractParameters extracts variables from variables.tf.
func (s *LocalTerraformSource) extractParameters(dirPath string) []RecipeParameter {
	var params []RecipeParameter

	varsFile := filepath.Join(dirPath, "variables.tf")
	content, err := os.ReadFile(varsFile)
	if err != nil {
		return params
	}

	// Simple regex to extract variable names and descriptions
	varPattern := regexp.MustCompile(`variable\s+"([^"]+)"\s*\{[^}]*description\s*=\s*"([^"]*)"`)
	matches := varPattern.FindAllStringSubmatch(string(content), -1)

	for _, match := range matches {
		if len(match) >= 3 {
			params = append(params, RecipeParameter{
				Name:        match[1],
				Description: match[2],
				Type:        "string",
				Required:    !strings.Contains(match[0], "default"),
			})
		}
	}

	return params
}
