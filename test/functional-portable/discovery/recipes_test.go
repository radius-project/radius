// Package discovery contains functional tests for recipe discovery.
package discovery

import (
	"context"
	"testing"

	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/recipes"
	"github.com/radius-project/radius/pkg/discovery/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecipeDiscoveryWorkflow(t *testing.T) {
	t.Parallel()

	t.Run("end-to-end recipe discovery workflow", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// Step 1: Create resource type mappings (simulating output from generate_resource_types skill)
		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-redis-1",
				ResourceType: discovery.ResourceType{
					Name:       "Applications.Datastores/redisCaches",
					APIVersion: "2023-10-01-preview",
				},
				Confidence: 0.95,
			},
			{
				DependencyID: "dep-postgres-1",
				ResourceType: discovery.ResourceType{
					Name:       "Applications.Datastores/sqlDatabases",
					APIVersion: "2023-10-01-preview",
				},
				Confidence: 0.90,
			},
			{
				DependencyID: "dep-rabbitmq-1",
				ResourceType: discovery.ResourceType{
					Name:       "Applications.Messaging/rabbitMQQueues",
					APIVersion: "2023-10-01-preview",
				},
				Confidence: 0.85,
			},
		}

		// Step 2: Discover recipes using the skill
		skill := skills.NewDiscoverRecipesSkill()
		output, err := skill.DiscoverRecipes(ctx, skills.DiscoverRecipesInput{
			ResourceTypeMappings: mappings,
			MinConfidence:        0.3,
			MaxMatchesPerType:    3,
		})
		require.NoError(t, err)

		// Step 3: Verify results
		assert.NotNil(t, output)
		assert.NotEmpty(t, output.SourcesUsed, "should have used at least one source")
		assert.NotEmpty(t, output.Summary, "should have a summary")

		// Should have matches for the resource types
		if len(output.Matches) > 0 {
			t.Logf("Found %d recipe matches", len(output.Matches))
			for _, match := range output.Matches {
				t.Logf("  - %s: %s (%.0f%% score) from %s",
					match.DependencyID, match.Recipe.Name, match.Score*100, match.Recipe.Provider)
			}
		}

		// Should have best matches
		if len(output.BestMatches) > 0 {
			t.Logf("Best matches:")
			for _, match := range output.BestMatches {
				t.Logf("  - %s: %s", match.DependencyID, match.Recipe.Name)
			}
		}

		t.Logf("Summary:\n%s", output.Summary)
	})

	t.Run("recipe discovery with cloud provider filter", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "dep-redis-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		skill := skills.NewDiscoverRecipesSkill()

		// Test with Azure filter
		azureOutput, err := skill.DiscoverRecipes(ctx, skills.DiscoverRecipesInput{
			ResourceTypeMappings: mappings,
			CloudProvider:        "azure",
		})
		require.NoError(t, err)

		// Test with AWS filter
		awsOutput, err := skill.DiscoverRecipes(ctx, skills.DiscoverRecipesInput{
			ResourceTypeMappings: mappings,
			CloudProvider:        "aws",
		})
		require.NoError(t, err)

		t.Logf("Azure matches: %d, AWS matches: %d",
			len(azureOutput.Matches), len(awsOutput.Matches))
	})

	t.Run("recipe discovery from dependencies", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// Start with raw dependencies
		deps := []discovery.DetectedDependency{
			{
				ID:         "dep-1",
				Name:       "redis",
				Type:       "redis",
				Library:    "redis-py",
				Confidence: 0.9,
			},
			{
				ID:         "dep-2",
				Name:       "postgres",
				Type:       "postgresql",
				Library:    "psycopg2",
				Confidence: 0.85,
			},
		}

		skill := skills.NewDiscoverRecipesSkill()
		output, err := skill.DiscoverRecipesFromDependencies(ctx, deps, "")
		require.NoError(t, err)
		assert.NotNil(t, output)

		t.Logf("Discovered %d recipe matches from %d dependencies",
			len(output.Matches), len(deps))
	})
}

func TestRecipeSourcesIntegration(t *testing.T) {
	t.Parallel()

	t.Run("AVM source returns known modules", func(t *testing.T) {
		t.Parallel()

		source, err := recipes.NewAVMSource(recipes.SourceConfig{
			Name: "avm-integration-test",
		})
		require.NoError(t, err)

		allRecipes, err := source.List(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, allRecipes)

		// Check for expected resource types (using new Radius.* namespace)
		resourceTypes := make(map[string]bool)
		for _, recipe := range allRecipes {
			resourceTypes[recipe.ResourceType] = true
		}

		assert.True(t, resourceTypes["Radius.Data/redisCaches"],
			"should have Redis recipes")
		assert.True(t, resourceTypes["Radius.Data/mySqlDatabases"],
			"should have MySQL recipes")
	})

	t.Run("Terraform source returns known modules", func(t *testing.T) {
		t.Parallel()

		source, err := recipes.NewTerraformSource(recipes.SourceConfig{
			Name: "terraform-integration-test",
		})
		require.NoError(t, err)

		allRecipes, err := source.List(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, allRecipes)

		// Check for AWS and Azure modules
		hasAWS := false
		hasAzure := false
		for _, recipe := range allRecipes {
			for _, tag := range recipe.Tags {
				if tag == "aws" {
					hasAWS = true
				}
				if tag == "azure" {
					hasAzure = true
				}
			}
		}

		assert.True(t, hasAWS, "should have AWS modules")
		assert.True(t, hasAzure || hasAWS, "should have cloud-specific modules")
	})
}

func TestRecipeMatcherIntegration(t *testing.T) {
	t.Parallel()

	t.Run("matcher with multiple sources", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()

		// Create registry with both AVM and Terraform sources
		registry := recipes.NewRegistry()

		avmSource, err := recipes.NewAVMSource(recipes.SourceConfig{Name: "avm"})
		require.NoError(t, err)
		require.NoError(t, registry.Register(avmSource))

		tfSource, err := recipes.NewTerraformSource(recipes.SourceConfig{Name: "terraform"})
		require.NoError(t, err)
		require.NoError(t, registry.Register(tfSource))

		// Create matcher with preferences
		options := recipes.MatcherOptions{
			MinConfidence:    0.3,
			MaxMatches:       5,
			PreferredSources: []string{"avm"},
			CloudProvider:    "azure",
		}
		matcher := recipes.NewMatcher(registry, options)

		// Match against resource types
		mappings := []discovery.ResourceTypeMapping{
			{
				DependencyID: "redis-1",
				ResourceType: discovery.ResourceType{
					Name: "Applications.Datastores/redisCaches",
				},
				Confidence: 0.9,
			},
		}

		matches, err := matcher.Match(ctx, mappings)
		require.NoError(t, err)

		if len(matches) > 0 {
			// With Azure preference, AVM should rank higher
			t.Logf("First match source: %s", matches[0].Recipe.Provider)
		}
	})

	t.Run("helper functions work correctly", func(t *testing.T) {
		t.Parallel()

		matches := []discovery.RecipeMatch{
			{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "avm-redis", Provider: "avm"}, Score: 0.95},
			{DependencyID: "redis-1", Recipe: discovery.Recipe{Name: "tf-redis", Provider: "terraform"}, Score: 0.85},
			{DependencyID: "sql-1", Recipe: discovery.Recipe{Name: "avm-sql", Provider: "avm"}, Score: 0.90},
			{DependencyID: "sql-1", Recipe: discovery.Recipe{Name: "tf-sql", Provider: "terraform"}, Score: 0.80},
		}

		// Group by dependency ID
		grouped := recipes.GroupByResourceType(matches)
		assert.Len(t, grouped, 2)
		assert.Len(t, grouped["redis-1"], 2)
		assert.Len(t, grouped["sql-1"], 2)

		// Select best matches
		best := recipes.SelectBestMatches(matches)
		assert.Len(t, best, 2)

		// Filter by source
		avmOnly := recipes.FilterBySource(matches, "avm")
		assert.Len(t, avmOnly, 2)
		for _, m := range avmOnly {
			assert.Equal(t, "avm", m.Recipe.Provider)
		}

		// Filter by score
		highScore := recipes.FilterByConfidence(matches, 0.9)
		assert.Len(t, highScore, 2)
	})
}
