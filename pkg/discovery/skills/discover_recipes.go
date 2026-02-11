// Package skills provides composable discovery skills.
package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/recipes"
)

// DiscoverRecipesSkill matches discovered dependencies to Radius recipes.
type DiscoverRecipesSkill struct {
	registry *recipes.Registry
}

// DiscoverRecipesInput is the input for the discover recipes skill.
type DiscoverRecipesInput struct {
	// ResourceTypeMappings are the resource types to match recipes for.
	ResourceTypeMappings []discovery.ResourceTypeMapping `json:"resourceTypeMappings"`

	// Sources lists recipe source configurations.
	Sources []recipes.SourceConfig `json:"sources,omitempty"`

	// MinConfidence is the minimum confidence threshold.
	MinConfidence float64 `json:"minConfidence,omitempty"`

	// MaxMatchesPerType is the maximum matches per resource type.
	MaxMatchesPerType int `json:"maxMatchesPerType,omitempty"`

	// CloudProvider filters by cloud provider (aws, azure, gcp).
	CloudProvider string `json:"cloudProvider,omitempty"`

	// PreferredSources lists source names to prefer.
	PreferredSources []string `json:"preferredSources,omitempty"`
}

// DiscoverRecipesOutput is the output from the discover recipes skill.
type DiscoverRecipesOutput struct {
	// Matches contains all recipe matches.
	Matches []discovery.RecipeMatch `json:"matches"`

	// BestMatches contains the best match for each resource type.
	BestMatches []discovery.RecipeMatch `json:"bestMatches"`

	// SourcesUsed lists the sources that were searched.
	SourcesUsed []string `json:"sourcesUsed"`

	// Summary provides a human-readable summary.
	Summary string `json:"summary"`
}

// NewDiscoverRecipesSkill creates a new discover recipes skill.
func NewDiscoverRecipesSkill() *DiscoverRecipesSkill {
	return &DiscoverRecipesSkill{
		registry: recipes.NewRegistry(),
	}
}

// Name returns the skill name.
func (s *DiscoverRecipesSkill) Name() string {
	return "discover_recipes"
}

// Description returns the skill description.
func (s *DiscoverRecipesSkill) Description() string {
	return "Matches discovered infrastructure dependencies to available Radius recipes from various sources"
}

// Execute runs the skill.
func (s *DiscoverRecipesSkill) Execute(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
	var in DiscoverRecipesInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, fmt.Errorf("parsing input: %w", err)
	}

	output, err := s.DiscoverRecipes(ctx, in)
	if err != nil {
		return nil, err
	}

	return json.Marshal(output)
}

// DiscoverRecipes performs recipe discovery.
func (s *DiscoverRecipesSkill) DiscoverRecipes(ctx context.Context, input DiscoverRecipesInput) (*DiscoverRecipesOutput, error) {
	// Create registry with configured sources
	registry := recipes.NewRegistry()

	// Add default sources if none configured
	sources := input.Sources
	if len(sources) == 0 {
		sources = s.getDefaultSources()
	}

	var sourcesUsed []string
	for _, config := range sources {
		source, err := recipes.CreateSource(config)
		if err != nil {
			continue // Skip failed sources
		}
		if err := registry.Register(source); err != nil {
			continue
		}
		sourcesUsed = append(sourcesUsed, config.Name)
	}

	// Configure matcher
	options := recipes.MatcherOptions{
		MinConfidence:    input.MinConfidence,
		MaxMatches:       input.MaxMatchesPerType,
		PreferredSources: input.PreferredSources,
		CloudProvider:    input.CloudProvider,
	}

	if options.MinConfidence == 0 {
		options.MinConfidence = 0.3
	}
	if options.MaxMatches == 0 {
		options.MaxMatches = 5
	}

	// Match recipes
	matcher := recipes.NewMatcher(registry, options)
	matches, err := matcher.Match(ctx, input.ResourceTypeMappings)
	if err != nil {
		return nil, fmt.Errorf("matching recipes: %w", err)
	}

	// Select best matches
	bestMatches := recipes.SelectBestMatches(matches)

	// Generate summary
	summary := s.generateSummary(matches, bestMatches, sourcesUsed)

	return &DiscoverRecipesOutput{
		Matches:     matches,
		BestMatches: bestMatches,
		SourcesUsed: sourcesUsed,
		Summary:     summary,
	}, nil
}

func (s *DiscoverRecipesSkill) getDefaultSources() []recipes.SourceConfig {
	return []recipes.SourceConfig{
		{
			Name: "local-terraform",
			Type: "local-terraform",
		},
		{
			Name: "avm-default",
			Type: "avm",
		},
		{
			Name: "terraform-default",
			Type: "terraform",
		},
	}
}

func (s *DiscoverRecipesSkill) generateSummary(matches, bestMatches []discovery.RecipeMatch, sources []string) string {
	if len(matches) == 0 {
		return "No recipe matches found. Consider adding custom recipe sources."
	}

	grouped := recipes.GroupByResourceType(matches)

	summary := fmt.Sprintf("Found %d recipe matches across %d resource types from %d sources.\n\n",
		len(matches), len(grouped), len(sources))

	summary += "Best matches:\n"
	for _, match := range bestMatches {
		summary += fmt.Sprintf("  - %s: %s (%.0f%% score) from %s\n",
			match.DependencyID, match.Recipe.Name, match.Score*100, match.Recipe.Provider)
	}

	return summary
}

// DiscoverRecipesFromDependencies is a convenience method that creates resource type mappings from dependencies.
// It uses a simple mapping from dependency types to resource types.
func (s *DiscoverRecipesSkill) DiscoverRecipesFromDependencies(ctx context.Context, deps []discovery.DetectedDependency, cloudProvider string) (*DiscoverRecipesOutput, error) {
	// Create resource type mappings from dependencies using a simple mapping
	var mappings []discovery.ResourceTypeMapping
	for _, dep := range deps {
		resourceType := mapDependencyToResourceType(dep.Type)
		if resourceType != "" {
			mappings = append(mappings, discovery.ResourceTypeMapping{
				DependencyID: dep.ID,
				ResourceType: discovery.ResourceType{
					Name: resourceType,
				},
				Confidence: dep.Confidence,
			})
		}
	}

	// Then, discover recipes
	return s.DiscoverRecipes(ctx, DiscoverRecipesInput{
		ResourceTypeMappings: mappings,
		CloudProvider:        cloudProvider,
	})
}

// mapDependencyToResourceType maps a dependency type to a Radius resource type.
func mapDependencyToResourceType(depType discovery.DependencyType) string {
	// Use the new Radius.* namespace from resource-types-contrib
	// See: https://github.com/radius-project/resource-types-contrib
	typeMap := map[discovery.DependencyType]string{
		discovery.DependencyRedis:      "Radius.Data/redisCaches",
		discovery.DependencyMongoDB:    "Radius.Data/mongoDatabases",
		discovery.DependencyPostgreSQL: "Radius.Data/postgreSqlDatabases",
		discovery.DependencyMySQL:      "Radius.Data/mySqlDatabases",
		discovery.DependencyRabbitMQ:   "Radius.Messaging/rabbitMQQueues",
		discovery.DependencyKafka:      "Radius.Messaging/kafkaQueues",
	}

	return typeMap[depType]
}

// RecipeRecommendation provides a structured recipe recommendation.
type RecipeRecommendation struct {
	// ResourceType is the Radius resource type.
	ResourceType string `json:"resourceType"`

	// DependencyName is the original dependency name.
	DependencyName string `json:"dependencyName"`

	// RecommendedRecipe is the recommended recipe.
	RecommendedRecipe discovery.RecipeMatch `json:"recommendedRecipe"`

	// Alternatives are alternative recipes.
	Alternatives []discovery.RecipeMatch `json:"alternatives"`

	// Rationale explains why this recipe was recommended.
	Rationale string `json:"rationale"`
}

// GetRecommendations returns structured recommendations for each resource type.
func (s *DiscoverRecipesSkill) GetRecommendations(matches []discovery.RecipeMatch, mappings []discovery.ResourceTypeMapping) []RecipeRecommendation {
	grouped := recipes.GroupByResourceType(matches)

	// Create a map of dependency IDs to mappings
	mappingsByID := make(map[string]discovery.ResourceTypeMapping)
	for _, m := range mappings {
		mappingsByID[m.DependencyID] = m
	}

	var recommendations []RecipeRecommendation
	for dependencyID, group := range grouped {
		if len(group) == 0 {
			continue
		}

		mapping, ok := mappingsByID[dependencyID]
		if !ok {
			continue
		}

		rec := RecipeRecommendation{
			ResourceType:      mapping.ResourceType.Name,
			DependencyName:    dependencyID,
			RecommendedRecipe: group[0],
			Rationale:         s.generateRationale(group[0], mapping),
		}

		if len(group) > 1 {
			rec.Alternatives = group[1:]
		}

		recommendations = append(recommendations, rec)
	}

	return recommendations
}

func (s *DiscoverRecipesSkill) generateRationale(match discovery.RecipeMatch, mapping discovery.ResourceTypeMapping) string {
	score := match.Score * 100

	if score >= 90 {
		return fmt.Sprintf("High confidence match (%.0f%%) for %s from %s",
			score, mapping.ResourceType.Name, match.Recipe.Provider)
	} else if score >= 70 {
		return fmt.Sprintf("Good match (%.0f%%) for %s. Consider reviewing configuration options.",
			score, mapping.ResourceType.Name)
	} else {
		return fmt.Sprintf("Potential match (%.0f%%). Manual verification recommended.",
			score)
	}
}
