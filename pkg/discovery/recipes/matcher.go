// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/discovery"
)

// Matcher matches discovered dependencies to available recipes.
type Matcher struct {
	registry *Registry
	options  MatcherOptions
}

// MatcherOptions configures the recipe matcher.
type MatcherOptions struct {
	// MinConfidence is the minimum confidence threshold for matches.
	MinConfidence float64

	// MaxMatches is the maximum number of matches per resource type.
	MaxMatches int

	// PreferredSources lists source names to prefer in order.
	PreferredSources []string

	// PreferredTags lists tags to prefer.
	PreferredTags []string

	// CloudProvider filters recipes by cloud provider (aws, azure, gcp).
	CloudProvider string
}

// DefaultMatcherOptions returns default matcher options.
func DefaultMatcherOptions() MatcherOptions {
	return MatcherOptions{
		MinConfidence: 0.3,
		MaxMatches:    5,
	}
}

// NewMatcher creates a new recipe matcher.
func NewMatcher(registry *Registry, options MatcherOptions) *Matcher {
	return &Matcher{
		registry: registry,
		options:  options,
	}
}

// Match finds recipes for the given resource type mappings.
func (m *Matcher) Match(ctx context.Context, mappings []discovery.ResourceTypeMapping) ([]discovery.RecipeMatch, error) {
	var allMatches []discovery.RecipeMatch

	for _, mapping := range mappings {
		matches, err := m.matchResourceType(ctx, mapping)
		if err != nil {
			continue
		}
		allMatches = append(allMatches, matches...)
	}

	return allMatches, nil
}

// MatchSingle finds recipes for a single resource type.
func (m *Matcher) MatchSingle(ctx context.Context, resourceType string) ([]RecipeMatch, error) {
	recipes, err := m.registry.SearchAll(ctx, resourceType)
	if err != nil {
		return nil, err
	}

	var matches []RecipeMatch
	for _, recipe := range recipes {
		score := m.scoreRecipe(recipe, resourceType)
		if score >= m.options.MinConfidence {
			matches = append(matches, RecipeMatch{
				Recipe: recipe,
				Score:  score,
				Reason: m.explainMatch(recipe, resourceType),
			})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Limit results
	if m.options.MaxMatches > 0 && len(matches) > m.options.MaxMatches {
		matches = matches[:m.options.MaxMatches]
	}

	return matches, nil
}

// RecipeMatch represents a recipe match with scoring.
type RecipeMatch struct {
	Recipe Recipe  `json:"recipe"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func (m *Matcher) matchResourceType(ctx context.Context, mapping discovery.ResourceTypeMapping) ([]discovery.RecipeMatch, error) {
	recipes, err := m.registry.SearchAll(ctx, mapping.ResourceType.Name)
	if err != nil {
		return nil, err
	}

	var matches []discovery.RecipeMatch
	for _, recipe := range recipes {
		// Filter by cloud provider if specified
		if m.options.CloudProvider != "" && !m.recipeMatchesProvider(recipe) {
			continue
		}

		score := m.scoreRecipe(recipe, mapping.ResourceType.Name)
		score *= mapping.Confidence // Weight by resource type confidence

		if score >= m.options.MinConfidence {
			matches = append(matches, discovery.RecipeMatch{
				DependencyID: mapping.DependencyID,
				Recipe: discovery.Recipe{
					Name:           recipe.Name,
					SourceType:     discovery.RecipeSourceType(recipe.SourceType),
					SourceLocation: recipe.TemplatePath,
					Version:        recipe.Version,
					Description:    recipe.Description,
					Provider:       recipe.Source,
				},
				Score:        score,
				MatchReasons: []string{m.explainMatch(recipe, mapping.ResourceType.Name)},
			})
		}
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Limit results per resource type
	if m.options.MaxMatches > 0 && len(matches) > m.options.MaxMatches {
		matches = matches[:m.options.MaxMatches]
	}

	return matches, nil
}

func (m *Matcher) scoreRecipe(recipe Recipe, resourceType string) float64 {
	score := 0.5 // Base score

	// Exact resource type match
	if recipe.ResourceType == resourceType {
		score += 0.3
	}

	// Preferred source bonus
	for i, source := range m.options.PreferredSources {
		if recipe.Source == source {
			// Earlier in list = higher bonus
			score += 0.1 * float64(len(m.options.PreferredSources)-i) / float64(len(m.options.PreferredSources))
			break
		}
	}

	// Preferred tags bonus
	for _, preferredTag := range m.options.PreferredTags {
		for _, tag := range recipe.Tags {
			if strings.EqualFold(tag, preferredTag) {
				score += 0.05
				break
			}
		}
	}

	// Cloud provider match
	if m.options.CloudProvider != "" {
		for _, tag := range recipe.Tags {
			if strings.EqualFold(tag, m.options.CloudProvider) {
				score += 0.15
				break
			}
		}
	}

	// Version bonus (versioned recipes are more reliable)
	if recipe.Version != "" {
		score += 0.05
	}

	// Description bonus (documented recipes are better)
	if recipe.Description != "" {
		score += 0.05
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// recipeMatchesProvider checks if a recipe matches the configured cloud provider.
// Local recipes always match. Other recipes must have a tag matching the provider.
func (m *Matcher) recipeMatchesProvider(recipe Recipe) bool {
	// Local recipes always match any provider
	if recipe.SourceType == "local-terraform" || recipe.Source == "local-terraform" {
		return true
	}

	// Check if recipe tags contain the cloud provider
	for _, tag := range recipe.Tags {
		if strings.EqualFold(tag, m.options.CloudProvider) {
			return true
		}
	}

	// AVM recipes only match azure provider
	if recipe.Source == "azure-verified-modules" && strings.EqualFold(m.options.CloudProvider, "azure") {
		return true
	}

	// resource-types-contrib recipes - check source type for provider
	if strings.Contains(recipe.SourceType, "resource-types-contrib") {
		// The source type includes the provider info
		return strings.Contains(strings.ToLower(recipe.SourceType), strings.ToLower(m.options.CloudProvider))
	}

	return false
}

func (m *Matcher) explainMatch(recipe Recipe, resourceType string) string {
	var reasons []string

	if recipe.ResourceType == resourceType {
		reasons = append(reasons, "exact resource type match")
	}

	for _, source := range m.options.PreferredSources {
		if recipe.Source == source {
			reasons = append(reasons, "from preferred source")
			break
		}
	}

	if m.options.CloudProvider != "" {
		for _, tag := range recipe.Tags {
			if strings.EqualFold(tag, m.options.CloudProvider) {
				reasons = append(reasons, "matches cloud provider")
				break
			}
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "general match")
	}

	return strings.Join(reasons, ", ")
}

// GroupByResourceType groups recipe matches by dependency ID.
func GroupByResourceType(matches []discovery.RecipeMatch) map[string][]discovery.RecipeMatch {
	grouped := make(map[string][]discovery.RecipeMatch)
	for _, match := range matches {
		grouped[match.DependencyID] = append(grouped[match.DependencyID], match)
	}
	return grouped
}

// SelectBestMatches selects the best recipe for each resource type.
func SelectBestMatches(matches []discovery.RecipeMatch) []discovery.RecipeMatch {
	grouped := GroupByResourceType(matches)
	var best []discovery.RecipeMatch

	for _, group := range grouped {
		if len(group) > 0 {
			// Assume already sorted by score
			best = append(best, group[0])
		}
	}

	return best
}

// FilterBySource filters matches to only include those from specific sources.
func FilterBySource(matches []discovery.RecipeMatch, sources ...string) []discovery.RecipeMatch {
	sourceSet := make(map[string]bool)
	for _, s := range sources {
		sourceSet[s] = true
	}

	var filtered []discovery.RecipeMatch
	for _, match := range matches {
		if sourceSet[match.Recipe.Provider] {
			filtered = append(filtered, match)
		}
	}

	return filtered
}

// FilterByConfidence filters matches to only include those above a threshold.
func FilterByConfidence(matches []discovery.RecipeMatch, threshold float64) []discovery.RecipeMatch {
	var filtered []discovery.RecipeMatch
	for _, match := range matches {
		if match.Score >= threshold {
			filtered = append(filtered, match)
		}
	}
	return filtered
}
