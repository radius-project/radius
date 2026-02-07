// Package skills provides composable discovery skills.
package skills

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverRecipesSkill(t *testing.T) {
	t.Parallel()

	t.Run("creates skill with name and description", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()
		assert.Equal(t, "discover_recipes", skill.Name())
		assert.NotEmpty(t, skill.Description())
	})

	t.Run("discovers recipes for resource types", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		input := DiscoverRecipesInput{
			ResourceTypeMappings: []discovery.ResourceTypeMapping{
				{
					DependencyID: "redis-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/redisCaches",
					},
					Confidence: 0.9,
				},
			},
		}

		output, err := skill.DiscoverRecipes(context.Background(), input)
		require.NoError(t, err)
		assert.NotNil(t, output)
		assert.NotEmpty(t, output.SourcesUsed)
		assert.NotEmpty(t, output.Summary)
	})

	t.Run("executes via JSON interface", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		input := DiscoverRecipesInput{
			ResourceTypeMappings: []discovery.ResourceTypeMapping{
				{
					DependencyID: "sql-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/sqlDatabases",
					},
					Confidence: 0.85,
				},
			},
			MinConfidence: 0.5,
		}

		inputJSON, err := json.Marshal(input)
		require.NoError(t, err)

		outputJSON, err := skill.Execute(context.Background(), inputJSON)
		require.NoError(t, err)

		var output DiscoverRecipesOutput
		err = json.Unmarshal(outputJSON, &output)
		require.NoError(t, err)
		assert.NotEmpty(t, output.SourcesUsed)
	})

	t.Run("filters by cloud provider", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		input := DiscoverRecipesInput{
			ResourceTypeMappings: []discovery.ResourceTypeMapping{
				{
					DependencyID: "redis-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/redisCaches",
					},
					Confidence: 0.9,
				},
			},
			CloudProvider: "azure",
		}

		output, err := skill.DiscoverRecipes(context.Background(), input)
		require.NoError(t, err)
		assert.NotNil(t, output)
	})

	t.Run("uses custom sources", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		input := DiscoverRecipesInput{
			ResourceTypeMappings: []discovery.ResourceTypeMapping{
				{
					DependencyID: "redis-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/redisCaches",
					},
					Confidence: 0.9,
				},
			},
			Sources: []recipes.SourceConfig{
				{Name: "custom-avm", Type: "avm"},
			},
		}

		output, err := skill.DiscoverRecipes(context.Background(), input)
		require.NoError(t, err)
		assert.Contains(t, output.SourcesUsed, "custom-avm")
	})

	t.Run("selects best matches", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		input := DiscoverRecipesInput{
			ResourceTypeMappings: []discovery.ResourceTypeMapping{
				{
					DependencyID: "redis-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/redisCaches",
					},
					Confidence: 0.9,
				},
				{
					DependencyID: "sql-1",
					ResourceType: discovery.ResourceType{
						Name: "Applications.Datastores/sqlDatabases",
					},
					Confidence: 0.85,
				},
			},
		}

		output, err := skill.DiscoverRecipes(context.Background(), input)
		require.NoError(t, err)

		// Should have at most one best match per resource type
		dependencyIDs := make(map[string]bool)
		for _, match := range output.BestMatches {
			assert.False(t, dependencyIDs[match.DependencyID], "duplicate dependency ID in best matches")
			dependencyIDs[match.DependencyID] = true
		}
	})
}

func TestGetRecommendations(t *testing.T) {
	t.Parallel()

	t.Run("generates recommendations from matches", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		matches := []discovery.RecipeMatch{
			{
				DependencyID: "redis-1",
				Recipe: discovery.Recipe{
					Name:     "avm-redis",
					Provider: "avm",
				},
				Score: 0.9,
			},
			{
				DependencyID: "redis-1",
				Recipe: discovery.Recipe{
					Name:     "tf-redis",
					Provider: "terraform",
				},
				Score: 0.7,
			},
		}

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "redis-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		recs := skill.GetRecommendations(matches, mappings)
		assert.Len(t, recs, 1)

		rec := recs[0]
		assert.Equal(t, "redis-1", rec.DependencyName)
		assert.Equal(t, "avm-redis", rec.RecommendedRecipe.Recipe.Name)
		assert.Len(t, rec.Alternatives, 1)
		assert.NotEmpty(t, rec.Rationale)
	})
}

func TestDiscoverRecipesFromDependencies(t *testing.T) {
	t.Parallel()

	t.Run("discovers recipes from dependencies", func(t *testing.T) {
		t.Parallel()

		skill := NewDiscoverRecipesSkill()

		deps := []discovery.DetectedDependency{
			{
				ID:   "redis-1",
				Name: "redis",
				Type: "redis",
			},
		}

		output, err := skill.DiscoverRecipesFromDependencies(context.Background(), deps, "azure")
		require.NoError(t, err)
		assert.NotNil(t, output)
	})
}
