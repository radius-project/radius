// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ContribSource discovers recipes from resource-types-contrib repository.
type ContribSource struct {
	name       string
	baseURL    string
	provider   string // kubernetes, azure, aws
	httpClient *http.Client
}

// ContribSourceConfig contains configuration for the contrib source.
type ContribSourceConfig struct {
	Name     string
	Provider string // kubernetes, azure, aws (defaults to kubernetes)
}

// NewContribSource creates a new resource-types-contrib recipe source.
func NewContribSource(config ContribSourceConfig) (*ContribSource, error) {
	provider := config.Provider
	if provider == "" {
		provider = "kubernetes"
	}

	return &ContribSource{
		name:     config.Name,
		baseURL:  "https://api.github.com/repos/radius-project/resource-types-contrib/contents",
		provider: provider,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the source name.
func (s *ContribSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *ContribSource) Type() string {
	return "contrib"
}

// Provider returns the cloud provider this source targets.
func (s *ContribSource) Provider() string {
	return s.provider
}

// Search searches for recipes matching the resource type.
func (s *ContribSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	// Parse resource type to get category and type name
	// e.g., "Radius.Data/mySqlDatabases" -> category: "Data", typeName: "mySqlDatabases"
	category, typeName := parseResourceType(resourceType)
	if category == "" || typeName == "" {
		return nil, nil
	}

	// Fetch recipes from contrib for this resource type
	recipes, err := s.fetchRecipesForType(ctx, category, typeName)
	if err != nil {
		return nil, err
	}

	return recipes, nil
}

// List lists all available recipes from this source.
func (s *ContribSource) List(ctx context.Context) ([]Recipe, error) {
	var allRecipes []Recipe

	// Get all categories from contrib
	categories, err := s.fetchCategories(ctx)
	if err != nil {
		return nil, err
	}

	for _, category := range categories {
		// Get resource types in this category
		types, err := s.fetchTypesInCategory(ctx, category)
		if err != nil {
			continue
		}

		for _, typeName := range types {
			recipes, err := s.fetchRecipesForType(ctx, category, typeName)
			if err != nil {
				continue
			}
			allRecipes = append(allRecipes, recipes...)
		}
	}

	return allRecipes, nil
}

// parseResourceType parses a resource type into category and type name.
// e.g., "Radius.Data/mySqlDatabases" -> ("Data", "mySqlDatabases")
func parseResourceType(resourceType string) (string, string) {
	parts := strings.Split(resourceType, "/")
	if len(parts) != 2 {
		return "", ""
	}

	namespace := parts[0]
	typeName := parts[1]

	// Extract category from namespace (e.g., "Radius.Data" -> "Data")
	nsParts := strings.Split(namespace, ".")
	if len(nsParts) < 2 {
		return "", ""
	}

	return nsParts[len(nsParts)-1], typeName
}

// fetchCategories fetches all categories from resource-types-contrib.
func (s *ContribSource) fetchCategories(ctx context.Context) ([]string, error) {
	url := s.baseURL
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch categories: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, err
	}

	// Filter for directories that are likely categories (not hidden, not docs)
	var categories []string
	for _, item := range contents {
		if item.Type == "dir" && !strings.HasPrefix(item.Name, ".") && item.Name != "docs" {
			categories = append(categories, item.Name)
		}
	}

	return categories, nil
}

// fetchTypesInCategory fetches all resource types in a category.
func (s *ContribSource) fetchTypesInCategory(ctx context.Context, category string) ([]string, error) {
	url := fmt.Sprintf("%s/%s", s.baseURL, category)
	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch types in %s: %d", category, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, err
	}

	var types []string
	for _, item := range contents {
		if item.Type == "dir" {
			types = append(types, item.Name)
		}
	}

	return types, nil
}

// fetchRecipesForType fetches recipes for a specific resource type.
func (s *ContribSource) fetchRecipesForType(ctx context.Context, category, typeName string) ([]Recipe, error) {
	// Path: <category>/<typeName>/recipes/<provider>
	recipesPath := fmt.Sprintf("%s/%s/%s/recipes/%s", s.baseURL, category, typeName, s.provider)

	resp, err := s.httpClient.Get(recipesPath)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// No recipes for this provider
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, err
	}

	var recipes []Recipe
	resourceType := fmt.Sprintf("Radius.%s/%s", category, typeName)

	// Get recipes from each template kind (bicep, terraform)
	for _, templateKind := range contents {
		if templateKind.Type == "dir" {
			kindRecipes, err := s.fetchRecipesOfKind(ctx, category, typeName, templateKind.Name)
			if err != nil {
				continue
			}
			for _, r := range kindRecipes {
				r.ResourceType = resourceType
				recipes = append(recipes, r)
			}
		}
	}

	return recipes, nil
}

// fetchRecipesOfKind fetches recipes of a specific template kind (bicep, terraform).
func (s *ContribSource) fetchRecipesOfKind(ctx context.Context, category, typeName, templateKind string) ([]Recipe, error) {
	// Path: <category>/<typeName>/recipes/<provider>/<templateKind>
	url := fmt.Sprintf("%s/%s/%s/recipes/%s/%s", s.baseURL, category, typeName, s.provider, templateKind)

	resp, err := s.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []struct {
		Name        string `json:"name"`
		Type        string `json:"type"`
		DownloadURL string `json:"download_url"`
	}
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, err
	}

	var recipes []Recipe
	for _, item := range contents {
		if item.Type != "file" {
			continue
		}

		// Filter by template kind
		isValidRecipe := false
		if templateKind == "bicep" && strings.HasSuffix(item.Name, ".bicep") {
			isValidRecipe = true
		} else if templateKind == "terraform" && (strings.HasSuffix(item.Name, ".tf") || item.Name == "main.tf") {
			isValidRecipe = true
		}

		if !isValidRecipe {
			continue
		}

		// Extract recipe name from filename
		recipeName := strings.TrimSuffix(item.Name, ".bicep")
		recipeName = strings.TrimSuffix(recipeName, ".tf")

		// Build the raw GitHub URL for the recipe template
		templatePath := fmt.Sprintf("https://raw.githubusercontent.com/radius-project/resource-types-contrib/main/%s/%s/recipes/%s/%s/%s",
			category, typeName, s.provider, templateKind, item.Name)

		recipes = append(recipes, Recipe{
			Name:         recipeName,
			Description:  fmt.Sprintf("%s recipe for %s on %s", templateKind, typeName, s.provider),
			Source:       "resource-types-contrib",
			SourceType:   templateKind,
			TemplatePath: templatePath,
			Tags:         []string{s.provider, templateKind, category},
		})
	}

	return recipes, nil
}

// contribRecipeLocations maps resource types to their recipe paths in resource-types-contrib.
// This is a quick lookup for known recipes without needing to query the GitHub API.
var contribRecipeLocations = map[string][]ContribRecipeInfo{
	"Radius.Data/mySqlDatabases": {
		{
			Provider:     "kubernetes",
			TemplateKind: "bicep",
			RecipeName:   "kubernetes-mysql",
			FileName:     "kubernetes-mysql.bicep",
		},
	},
	"Radius.Data/postgreSqlDatabases": {
		{
			Provider:     "kubernetes",
			TemplateKind: "bicep",
			RecipeName:   "kubernetes-postgresql",
			FileName:     "kubernetes-postgresql.bicep",
		},
	},
}

// ContribRecipeInfo contains information about a recipe in resource-types-contrib.
type ContribRecipeInfo struct {
	Provider     string // kubernetes, azure, aws
	TemplateKind string // bicep, terraform
	RecipeName   string
	FileName     string
}

// GetKnownRecipe returns recipe info if it exists in the known recipes map.
func GetKnownRecipe(resourceType, provider string) (*ContribRecipeInfo, bool) {
	recipes, exists := contribRecipeLocations[resourceType]
	if !exists {
		return nil, false
	}

	for _, r := range recipes {
		if r.Provider == provider {
			return &r, true
		}
	}

	return nil, false
}

// BuildContribRecipeURL builds the raw GitHub URL for a contrib recipe.
func BuildContribRecipeURL(resourceType string, info *ContribRecipeInfo) string {
	category, typeName := parseResourceType(resourceType)
	if category == "" || typeName == "" {
		return ""
	}

	return fmt.Sprintf("https://raw.githubusercontent.com/radius-project/resource-types-contrib/main/%s/%s/recipes/%s/%s/%s",
		category, typeName, info.Provider, info.TemplateKind, info.FileName)
}
