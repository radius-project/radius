// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatcher(t *testing.T) {
	t.Parallel()

	t.Run("matches recipes to resource types", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "redis-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "test-source",
					Version:      "1.0.0",
				},
			},
		}
		require.NoError(t, registry.Register(source))

		matcher := NewMatcher(registry, DefaultMatcherOptions())

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		matches, err := matcher.Match(context.Background(), mappings)
		require.NoError(t, err)
		assert.Len(t, matches, 1)
		assert.Equal(t, "redis-recipe", matches[0].Recipe.Name)
		assert.Equal(t, "test-source", matches[0].Recipe.Provider)
	})

	t.Run("filters by minimum confidence", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "weak-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "test-source",
				},
			},
		}
		require.NoError(t, registry.Register(source))

		options := MatcherOptions{
			MinConfidence: 0.9, // High threshold
			MaxMatches:    5,
		}
		matcher := NewMatcher(registry, options)

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.3, // Low confidence mapping
			},
		}

		matches, err := matcher.Match(context.Background(), mappings)
		require.NoError(t, err)
		assert.Empty(t, matches) // Should be filtered out
	})

	t.Run("limits matches per resource type", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{Name: "recipe-1", ResourceType: "Applications.Datastores/redisCaches", Source: "test-source"},
				{Name: "recipe-2", ResourceType: "Applications.Datastores/redisCaches", Source: "test-source"},
				{Name: "recipe-3", ResourceType: "Applications.Datastores/redisCaches", Source: "test-source"},
			},
		}
		require.NoError(t, registry.Register(source))

		options := MatcherOptions{
			MinConfidence: 0.1,
			MaxMatches:    2, // Limit to 2
		}
		matcher := NewMatcher(registry, options)

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		matches, err := matcher.Match(context.Background(), mappings)
		require.NoError(t, err)
		assert.Len(t, matches, 2)
	})

	t.Run("boosts preferred sources", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source1 := &testSource{
			name:       "preferred-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "preferred-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "preferred-source",
				},
			},
		}
		source2 := &testSource{
			name:       "other-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "other-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "other-source",
				},
			},
		}
		require.NoError(t, registry.Register(source1))
		require.NoError(t, registry.Register(source2))

		options := MatcherOptions{
			MinConfidence:    0.1,
			MaxMatches:       5,
			PreferredSources: []string{"preferred-source"},
		}
		matcher := NewMatcher(registry, options)

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		matches, err := matcher.Match(context.Background(), mappings)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(matches), 1)
		// First match should be from preferred source (higher score)
		assert.Equal(t, "preferred-source", matches[0].Recipe.Provider)
	})

	t.Run("filters by cloud provider", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "azure-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "test-source",
					Tags:         []string{"azure", "redis"},
				},
				{
					Name:         "aws-recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "test-source",
					Tags:         []string{"aws", "redis"},
				},
			},
		}
		require.NoError(t, registry.Register(source))

		options := MatcherOptions{
			MinConfidence: 0.1,
			MaxMatches:    5,
			CloudProvider: "azure",
		}
		matcher := NewMatcher(registry, options)

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		matches, err := matcher.Match(context.Background(), mappings)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(matches), 1)
		// Azure recipe should have higher score
		assert.Equal(t, "azure-recipe", matches[0].Recipe.Name)
	})
}

func TestMatchSingle(t *testing.T) {
	t.Parallel()

	t.Run("returns scored matches for resource type", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()
		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{
					Name:         "redis-cache",
					Description:  "A Redis cache recipe",
					ResourceType: "Applications.Datastores/redisCaches",
					Source:       "test-source",
					Version:      "1.0.0",
				},
			},
		}
		require.NoError(t, registry.Register(source))

		matcher := NewMatcher(registry, DefaultMatcherOptions())

		matches, err := matcher.MatchSingle(context.Background(), "Applications.Datastores/redisCaches")
		require.NoError(t, err)
		assert.Len(t, matches, 1)
		assert.Equal(t, "redis-cache", matches[0].Recipe.Name)
		assert.Greater(t, matches[0].Score, 0.0)
		assert.NotEmpty(t, matches[0].Reason)
	})
}

func TestGroupByResourceType(t *testing.T) {
	t.Parallel()

	matches := []discovery.RecipeMatch{
		{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "redis-recipe-1"}},
		{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "redis-recipe-2"}},
		{DependencyID: "sql-1", Recipe: discovery.Recipe{Name: "sql-recipe"}},
	}

	grouped := GroupByResourceType(matches)

	assert.Len(t, grouped, 2)
	assert.Len(t, grouped["redis-1"], 2)
	assert.Len(t, grouped["sql-1"], 1)
}

func TestSelectBestMatches(t *testing.T) {
	t.Parallel()

	matches := []discovery.RecipeMatch{
		{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "redis-recipe-1"}, Score: 0.9},
		{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "redis-recipe-2"}, Score: 0.7},
		{DependencyID: "sql-1", Recipe: discovery.Recipe{Name: "sql-recipe"}, Score: 0.8},
	}

	best := SelectBestMatches(matches)

	assert.Len(t, best, 2)
}

func TestFilterByConfidence(t *testing.T) {
	t.Parallel()

	matches := []discovery.RecipeMatch{
		{Recipe: discovery.Recipe{Name: "high"}, Score: 0.9},
		{Recipe: discovery.Recipe{Name: "medium"}, Score: 0.6},
		{Recipe: discovery.Recipe{Name: "low"}, Score: 0.3},
	}

	filtered := FilterByConfidence(matches, 0.5)

	assert.Len(t, filtered, 2)
	assert.Equal(t, "high", filtered[0].Recipe.Name)
	assert.Equal(t, "medium", filtered[1].Recipe.Name)
}

func TestFilterBySource(t *testing.T) {
	t.Parallel()

	matches := []discovery.RecipeMatch{
		{Recipe: discovery.Recipe{Name: "avm-1", Provider: "avm"}},
		{Recipe: discovery.Recipe{Name: "tf-1", Provider: "terraform"}},
		{Recipe: discovery.Recipe{Name: "avm-2", Provider: "avm"}},
	}

	filtered := FilterBySource(matches, "avm")

	assert.Len(t, filtered, 2)
	for _, m := range filtered {
		assert.Equal(t, "avm", m.Recipe.Provider)
	}
}
