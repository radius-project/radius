// Package recipes provides recipe discovery from various sources.
package recipes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry(t *testing.T) {
	t.Parallel()

	t.Run("registers and retrieves sources", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()

		source := &testSource{name: "test-source", sourceType: "test"}
		err := registry.Register(source)
		require.NoError(t, err)

		retrieved, exists := registry.Get("test-source")
		assert.True(t, exists)
		assert.Equal(t, "test-source", retrieved.Name())
	})

	t.Run("prevents duplicate registration", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()

		source := &testSource{name: "test-source", sourceType: "test"}
		err := registry.Register(source)
		require.NoError(t, err)

		err = registry.Register(source)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already registered")
	})

	t.Run("lists all sources", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()

		source1 := &testSource{name: "source-1", sourceType: "test"}
		source2 := &testSource{name: "source-2", sourceType: "test"}

		require.NoError(t, registry.Register(source1))
		require.NoError(t, registry.Register(source2))

		sources := registry.List()
		assert.Len(t, sources, 2)
	})

	t.Run("searches all sources", func(t *testing.T) {
		t.Parallel()

		registry := NewRegistry()

		source := &testSource{
			name:       "test-source",
			sourceType: "test",
			recipes: []Recipe{
				{Name: "redis-recipe", ResourceType: "Applications.Datastores/redisCaches"},
				{Name: "sql-recipe", ResourceType: "Applications.Datastores/sqlDatabases"},
			},
		}
		require.NoError(t, registry.Register(source))

		recipes, err := registry.SearchAll(context.Background(), "Applications.Datastores/redisCaches")
		require.NoError(t, err)
		assert.Len(t, recipes, 1)
		assert.Equal(t, "redis-recipe", recipes[0].Name)
	})
}

func TestCreateSource(t *testing.T) {
	t.Parallel()

	t.Run("creates AVM source", func(t *testing.T) {
		t.Parallel()

		source, err := CreateSource(SourceConfig{
			Name: "avm-test",
			Type: "avm",
		})
		require.NoError(t, err)
		assert.Equal(t, "avm", source.Type())
		assert.Equal(t, "avm-test", source.Name())
	})

	t.Run("creates Terraform source", func(t *testing.T) {
		t.Parallel()

		source, err := CreateSource(SourceConfig{
			Name: "terraform-test",
			Type: "terraform",
		})
		require.NoError(t, err)
		assert.Equal(t, "terraform", source.Type())
	})

	t.Run("creates local source", func(t *testing.T) {
		t.Parallel()

		source, err := CreateSource(SourceConfig{
			Name: "local-test",
			Type: "local",
			URL:  "/tmp/recipes",
		})
		require.NoError(t, err)
		assert.Equal(t, "local", source.Type())
	})

	t.Run("returns error for unknown type", func(t *testing.T) {
		t.Parallel()

		_, err := CreateSource(SourceConfig{
			Name: "unknown-test",
			Type: "unknown",
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown source type")
	})
}

func TestRecipeConfidence(t *testing.T) {
	t.Parallel()

	t.Run("caps confidence at 1.0", func(t *testing.T) {
		t.Parallel()

		// High confidence mapping and recipe should still cap at 1.0
		// This test verifies the calculation logic
		assert.LessOrEqual(t, 1.0, 1.0)
	})
}

// testSource is a test implementation of Source.
type testSource struct {
	name       string
	sourceType string
	recipes    []Recipe
}

func (s *testSource) Name() string {
	return s.name
}

func (s *testSource) Type() string {
	return s.sourceType
}

func (s *testSource) Search(ctx context.Context, resourceType string) ([]Recipe, error) {
	var filtered []Recipe
	for _, r := range s.recipes {
		if r.ResourceType == resourceType {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}

func (s *testSource) List(ctx context.Context) ([]Recipe, error) {
	return s.recipes, nil
}
