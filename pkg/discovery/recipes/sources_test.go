// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAVMSource(t *testing.T) {
	t.Parallel()

	t.Run("creates source with default URL", func(t *testing.T) {
		t.Parallel()

		source, err := NewAVMSource(SourceConfig{
			Name: "avm-test",
		})
		require.NoError(t, err)
		assert.Equal(t, "avm-test", source.Name())
		assert.Equal(t, "avm", source.Type())
	})

	t.Run("lists known modules", func(t *testing.T) {
		t.Parallel()

		source, err := NewAVMSource(SourceConfig{
			Name: "avm-test",
		})
		require.NoError(t, err)

		recipes, err := source.List(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, recipes)

		// Verify all recipes have required fields
		for _, recipe := range recipes {
			assert.NotEmpty(t, recipe.Name)
			assert.NotEmpty(t, recipe.ResourceType)
			assert.True(t, recipe.SourceType == "avm" || recipe.SourceType == "contrib", "source type should be avm or contrib")
		}
	})

	t.Run("searches for Redis recipes", func(t *testing.T) {
		t.Parallel()

		source, err := NewAVMSource(SourceConfig{
			Name: "avm-test",
		})
		require.NoError(t, err)

		// Test with new Radius.Data namespace
		recipes, err := source.Search(context.Background(), "Radius.Data/redisCaches")
		require.NoError(t, err)
		assert.NotEmpty(t, recipes)

		for _, recipe := range recipes {
			assert.Equal(t, "Radius.Data/redisCaches", recipe.ResourceType)
		}
	})

	t.Run("returns empty for unknown resource type", func(t *testing.T) {
		t.Parallel()

		source, err := NewAVMSource(SourceConfig{
			Name: "avm-test",
		})
		require.NoError(t, err)

		recipes, err := source.Search(context.Background(), "Unknown/ResourceType")
		require.NoError(t, err)
		assert.Empty(t, recipes)
	})
}

func TestTerraformSource(t *testing.T) {
	t.Parallel()

	t.Run("creates source with default URL", func(t *testing.T) {
		t.Parallel()

		source, err := NewTerraformSource(SourceConfig{
			Name: "terraform-test",
		})
		require.NoError(t, err)
		assert.Equal(t, "terraform-test", source.Name())
		assert.Equal(t, "terraform", source.Type())
	})

	t.Run("creates source with custom namespace", func(t *testing.T) {
		t.Parallel()

		source, err := NewTerraformSource(SourceConfig{
			Name: "terraform-test",
			Options: map[string]string{
				"namespace": "myorg",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "myorg", source.namespace)
	})

	t.Run("lists known modules", func(t *testing.T) {
		t.Parallel()

		source, err := NewTerraformSource(SourceConfig{
			Name: "terraform-test",
		})
		require.NoError(t, err)

		recipes, err := source.List(context.Background())
		require.NoError(t, err)
		assert.NotEmpty(t, recipes)

		// Verify all recipes have required fields
		for _, recipe := range recipes {
			assert.NotEmpty(t, recipe.Name)
			assert.NotEmpty(t, recipe.ResourceType)
			assert.Equal(t, "terraform", recipe.SourceType)
		}
	})
}

func TestLocalSource(t *testing.T) {
	t.Parallel()

	t.Run("creates source with path", func(t *testing.T) {
		t.Parallel()

		source, err := NewLocalSource(SourceConfig{
			Name: "local-test",
			URL:  "/tmp/recipes",
		})
		require.NoError(t, err)
		assert.Equal(t, "local-test", source.Name())
		assert.Equal(t, "local", source.Type())
	})

	t.Run("expands home directory", func(t *testing.T) {
		t.Parallel()

		source, err := NewLocalSource(SourceConfig{
			Name: "local-test",
			URL:  "~/recipes",
		})
		require.NoError(t, err)
		assert.NotContains(t, source.path, "~")
	})

	t.Run("returns error without path", func(t *testing.T) {
		t.Parallel()

		_, err := NewLocalSource(SourceConfig{
			Name: "local-test",
		})
		assert.Error(t, err)
	})
}

func TestGitSource(t *testing.T) {
	t.Parallel()

	t.Run("creates source with URL", func(t *testing.T) {
		t.Parallel()

		source, err := NewGitSource(SourceConfig{
			Name: "git-test",
			URL:  "https://github.com/example/recipes",
		})
		require.NoError(t, err)
		assert.Equal(t, "git-test", source.Name())
		assert.Equal(t, "git", source.Type())
	})

	t.Run("uses custom branch and path", func(t *testing.T) {
		t.Parallel()

		source, err := NewGitSource(SourceConfig{
			Name: "git-test",
			URL:  "https://github.com/example/recipes",
			Options: map[string]string{
				"branch": "develop",
				"path":   "custom/recipes",
			},
		})
		require.NoError(t, err)
		assert.Equal(t, "develop", source.branch)
		assert.Equal(t, "custom/recipes", source.path)
	})

	t.Run("returns error without URL", func(t *testing.T) {
		t.Parallel()

		_, err := NewGitSource(SourceConfig{
			Name: "git-test",
		})
		assert.Error(t, err)
	})
}
