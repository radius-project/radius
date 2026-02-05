// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// GitSource discovers recipes from a Git repository.
type GitSource struct {
	name       string
	repoURL    string
	branch     string
	path       string
	httpClient *http.Client
	token      string
}

// NewGitSource creates a new Git repository recipe source.
func NewGitSource(config SourceConfig) (*GitSource, error) {
	if config.URL == "" {
		return nil, fmt.Errorf("git source requires URL")
	}

	branch := "main"
	if b, ok := config.Options["branch"]; ok {
		branch = b
	}

	path := "recipes"
	if p, ok := config.Options["path"]; ok {
		path = p
	}

	var token string
	if config.Credentials != nil {
		token = config.Credentials.Token
	}

	return &GitSource{
		name:    config.Name,
		repoURL: config.URL,
		branch:  branch,
		path:    path,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// Name returns the source name.
func (s *GitSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *GitSource) Type() string {
	return "git"
}

// Search searches for recipes matching the resource type.
func (s *GitSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	allRecipes, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Recipe
	for _, recipe := range allRecipes {
		if recipe.ResourceType == resourceType {
			filtered = append(filtered, recipe)
		}
	}

	return filtered, nil
}

// List lists all available recipes from the Git repository.
func (s *GitSource) List(ctx context.Context) ([]Recipe, error) {
	// Try to fetch recipe index from the repository
	recipes, err := s.fetchRecipeIndex(ctx)
	if err != nil {
		// Fall back to scanning the repository
		return s.scanRepository(ctx)
	}

	return recipes, nil
}

// RecipeIndex represents a recipes index file.
type RecipeIndex struct {
	Version string        `yaml:"version" json:"version"`
	Recipes []RecipeEntry `yaml:"recipes" json:"recipes"`
}

// RecipeEntry represents a recipe entry in the index.
type RecipeEntry struct {
	Name         string            `yaml:"name" json:"name"`
	Description  string            `yaml:"description" json:"description"`
	ResourceType string            `yaml:"resourceType" json:"resourceType"`
	Path         string            `yaml:"path" json:"path"`
	Version      string            `yaml:"version" json:"version"`
	Parameters   []RecipeParameter `yaml:"parameters" json:"parameters"`
	Tags         []string          `yaml:"tags" json:"tags"`
}

func (s *GitSource) fetchRecipeIndex(ctx context.Context) ([]Recipe, error) {
	// Try to fetch recipes.yaml or recipes.json from the repository
	indexURL := s.buildRawURL("recipes.yaml")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, err
	}

	if s.token != "" {
		req.Header.Set("Authorization", "token "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("index not found: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var index RecipeIndex
	if err := yaml.Unmarshal(body, &index); err != nil {
		return nil, err
	}

	var recipes []Recipe
	for _, entry := range index.Recipes {
		recipes = append(recipes, Recipe{
			Name:         entry.Name,
			Description:  entry.Description,
			ResourceType: entry.ResourceType,
			Source:       s.name,
			SourceType:   "git",
			Version:      entry.Version,
			TemplatePath: s.buildRawURL(entry.Path),
			Parameters:   entry.Parameters,
			Tags:         entry.Tags,
		})
	}

	return recipes, nil
}

func (s *GitSource) scanRepository(ctx context.Context) ([]Recipe, error) {
	// For GitHub, use the API to list contents
	if strings.Contains(s.repoURL, "github.com") {
		return s.scanGitHubRepository(ctx)
	}

	// For other Git hosts, return empty
	return nil, nil
}

func (s *GitSource) scanGitHubRepository(ctx context.Context) ([]Recipe, error) {
	// Extract owner and repo from URL
	parts := strings.Split(strings.TrimSuffix(s.repoURL, ".git"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL")
	}
	owner := parts[len(parts)-2]
	repo := parts[len(parts)-1]

	// Use GitHub API to list contents
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		owner, repo, s.path, s.branch)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	if s.token != "" {
		req.Header.Set("Authorization", "token "+s.token)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list contents: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var contents []GitHubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, err
	}

	var recipes []Recipe
	for _, content := range contents {
		if content.Type == "dir" {
			// This could be a recipe directory
			recipe := Recipe{
				Name:         content.Name,
				Description:  fmt.Sprintf("Recipe from %s", s.name),
				Source:       s.name,
				SourceType:   "git",
				TemplatePath: content.DownloadURL,
				Tags:         []string{"git", repo},
			}
			recipes = append(recipes, recipe)
		}
	}

	return recipes, nil
}

// GitHubContent represents a GitHub API content response.
type GitHubContent struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	DownloadURL string `json:"download_url"`
}

func (s *GitSource) buildRawURL(path string) string {
	// Convert GitHub URL to raw content URL
	if strings.Contains(s.repoURL, "github.com") {
		repoPath := strings.TrimPrefix(s.repoURL, "https://github.com/")
		repoPath = strings.TrimSuffix(repoPath, ".git")
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s/%s",
			repoPath, s.branch, s.path, path)
	}

	// Generic raw URL construction
	return fmt.Sprintf("%s/raw/%s/%s/%s", s.repoURL, s.branch, s.path, path)
}

// LocalSource discovers recipes from a local directory.
type LocalSource struct {
	name string
	path string
}

// NewLocalSource creates a new local directory recipe source.
func NewLocalSource(config SourceConfig) (*LocalSource, error) {
	path := config.URL
	if path == "" {
		return nil, fmt.Errorf("local source requires path in URL field")
	}

	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[1:])
	}

	return &LocalSource{
		name: config.Name,
		path: path,
	}, nil
}

// Name returns the source name.
func (s *LocalSource) Name() string {
	return s.name
}

// Type returns the source type.
func (s *LocalSource) Type() string {
	return "local"
}

// Search searches for recipes matching the resource type.
func (s *LocalSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	allRecipes, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []Recipe
	for _, recipe := range allRecipes {
		if recipe.ResourceType == resourceType {
			filtered = append(filtered, recipe)
		}
	}

	return filtered, nil
}

// List lists all available recipes from the local directory.
func (s *LocalSource) List(ctx context.Context) ([]Recipe, error) {
	// Try to read recipes index
	indexPath := filepath.Join(s.path, "recipes.yaml")
	data, err := os.ReadFile(indexPath)
	if err == nil {
		var index RecipeIndex
		if err := yaml.Unmarshal(data, &index); err == nil {
			var recipes []Recipe
			for _, entry := range index.Recipes {
				recipes = append(recipes, Recipe{
					Name:         entry.Name,
					Description:  entry.Description,
					ResourceType: entry.ResourceType,
					Source:       s.name,
					SourceType:   "local",
					Version:      entry.Version,
					TemplatePath: filepath.Join(s.path, entry.Path),
					Parameters:   entry.Parameters,
					Tags:         entry.Tags,
				})
			}
			return recipes, nil
		}
	}

	// Fall back to scanning directory
	return s.scanDirectory()
}

func (s *LocalSource) scanDirectory() ([]Recipe, error) {
	var recipes []Recipe

	entries, err := os.ReadDir(s.path)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		recipePath := filepath.Join(s.path, entry.Name())

		// Look for recipe definition file
		metaPath := filepath.Join(recipePath, "recipe.yaml")
		if _, err := os.Stat(metaPath); err == nil {
			recipe, err := s.loadRecipeFromMeta(metaPath, entry.Name())
			if err == nil {
				recipes = append(recipes, recipe)
				continue
			}
		}

		// Default recipe from directory
		recipes = append(recipes, Recipe{
			Name:         entry.Name(),
			Description:  fmt.Sprintf("Local recipe: %s", entry.Name()),
			Source:       s.name,
			SourceType:   "local",
			TemplatePath: recipePath,
			Tags:         []string{"local"},
		})
	}

	return recipes, nil
}

func (s *LocalSource) loadRecipeFromMeta(metaPath, name string) (Recipe, error) {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return Recipe{}, err
	}

	var entry RecipeEntry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return Recipe{}, err
	}

	return Recipe{
		Name:         entry.Name,
		Description:  entry.Description,
		ResourceType: entry.ResourceType,
		Source:       s.name,
		SourceType:   "local",
		Version:      entry.Version,
		TemplatePath: filepath.Dir(metaPath),
		Parameters:   entry.Parameters,
		Tags:         entry.Tags,
	}, nil
}
