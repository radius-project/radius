// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
)

// Source represents a source of Radius recipes.
type Source interface {
	// Name returns the source name.
	Name() string

	// Type returns the source type (avm, terraform, git, local).
	Type() string

	// Search searches for recipes matching the given resource type.
	Search(ctx context.Context, resourceType string) ([]Recipe, error)

	// List lists all available recipes from this source.
	List(ctx context.Context) ([]Recipe, error)
}

// Recipe represents a discovered recipe.
type Recipe struct {
	// Name is the recipe name.
	Name string `json:"name"`

	// Description describes the recipe.
	Description string `json:"description"`

	// ResourceType is the Radius resource type this recipe provisions.
	ResourceType string `json:"resourceType"`

	// Source is the source where this recipe was found.
	Source string `json:"source"`

	// SourceType is the type of source (avm, terraform, git, local).
	SourceType string `json:"sourceType"`

	// Version is the recipe version.
	Version string `json:"version"`

	// TemplatePath is the path or reference to the recipe template.
	TemplatePath string `json:"templatePath"`

	// Parameters are the configurable parameters for this recipe.
	Parameters []RecipeParameter `json:"parameters,omitempty"`

	// Tags are metadata tags for categorization.
	Tags []string `json:"tags,omitempty"`
}

// RecipeParameter describes a recipe parameter.
type RecipeParameter struct {
	// Name is the parameter name.
	Name string `json:"name"`

	// Type is the parameter type.
	Type string `json:"type"`

	// Description describes the parameter.
	Description string `json:"description"`

	// Required indicates if the parameter is required.
	Required bool `json:"required"`

	// Default is the default value.
	Default interface{} `json:"default,omitempty"`
}

// SourceConfig contains configuration for a recipe source.
type SourceConfig struct {
	// Name is the source name.
	Name string `json:"name"`

	// Type is the source type (avm, terraform, git, local).
	Type string `json:"type"`

	// URL is the source URL or path.
	URL string `json:"url"`

	// Credentials are optional authentication credentials.
	Credentials *SourceCredentials `json:"credentials,omitempty"`

	// Options are source-specific options.
	Options map[string]string `json:"options,omitempty"`
}

// SourceCredentials contains authentication credentials.
type SourceCredentials struct {
	// Token is an access token.
	Token string `json:"token,omitempty"`

	// Username for basic auth.
	Username string `json:"username,omitempty"`

	// Password for basic auth.
	Password string `json:"password,omitempty"`
}

// Registry manages multiple recipe sources.
type Registry struct {
	sources map[string]Source
}

// NewRegistry creates a new recipe source registry.
func NewRegistry() *Registry {
	return &Registry{
		sources: make(map[string]Source),
	}
}

// Register registers a recipe source.
func (r *Registry) Register(source Source) error {
	if _, exists := r.sources[source.Name()]; exists {
		return fmt.Errorf("source %q already registered", source.Name())
	}
	r.sources[source.Name()] = source
	return nil
}

// Get returns a source by name.
func (r *Registry) Get(name string) (Source, bool) {
	source, exists := r.sources[name]
	return source, exists
}

// List returns all registered sources.
func (r *Registry) List() []Source {
	sources := make([]Source, 0, len(r.sources))
	for _, source := range r.sources {
		sources = append(sources, source)
	}
	return sources
}

// SearchAll searches all sources for recipes matching the resource type.
func (r *Registry) SearchAll(ctx context.Context, resourceType string) ([]Recipe, error) {
	var allRecipes []Recipe
	for _, source := range r.sources {
		recipes, err := source.Search(ctx, resourceType)
		if err != nil {
			continue // Skip failed sources
		}
		allRecipes = append(allRecipes, recipes...)
	}
	return allRecipes, nil
}

// MatchRecipes matches resource types to available recipes.
func (r *Registry) MatchRecipes(ctx context.Context, resourceTypes []discovery.ResourceTypeMapping) ([]discovery.RecipeMatch, error) {
	var matches []discovery.RecipeMatch

	for _, rt := range resourceTypes {
		recipes, err := r.SearchAll(ctx, rt.ResourceType.Name)
		if err != nil {
			continue
		}

		for _, recipe := range recipes {
			confidence := calculateRecipeConfidence(rt, recipe)
			match := discovery.RecipeMatch{
				DependencyID: rt.DependencyID,
				Recipe: discovery.Recipe{
					Name:           recipe.Name,
					SourceType:     discovery.RecipeSourceType(recipe.SourceType),
					SourceLocation: recipe.TemplatePath,
					Version:        recipe.Version,
					Description:    recipe.Description,
					Provider:       recipe.Source,
				},
				Score:        confidence,
				MatchReasons: []string{"matched by resource type"},
			}
			matches = append(matches, match)
		}
	}

	return matches, nil
}

func calculateRecipeConfidence(rt discovery.ResourceTypeMapping, recipe Recipe) float64 {
	// Base confidence from resource type matching
	confidence := rt.Confidence * 0.8

	// Boost for exact resource type match
	if recipe.ResourceType == rt.ResourceType.Name {
		confidence += 0.15
	}

	// Slight boost for versioned recipes
	if recipe.Version != "" {
		confidence += 0.05
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// CreateSource creates a source from configuration.
func CreateSource(config SourceConfig) (Source, error) {
	switch config.Type {
	case "avm":
		return NewAVMSource(config)
	case "terraform":
		return NewTerraformSource(config)
	case "git":
		return NewGitSource(config)
	case "local":
		return NewLocalSource(config)
	case "local-terraform":
		return NewLocalTerraformSource(config)
	default:
		return nil, fmt.Errorf("unknown source type: %s", config.Type)
	}
}
