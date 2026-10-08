/*
Copyright 2023 The Radius Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package defaults

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
)

// recipePackResourceTypePrefix identifies the recipe pack resource in a compiled
// recipe pack template, independent of its API version.
const recipePackResourceTypePrefix = "Radius.Core/recipePacks@"

// RecipePackRecipe is one recipe in a recipe pack.
type RecipePackRecipe struct {
	// ResourceType is the fully qualified resource type the recipe serves, e.g.
	// "Radius.Compute/containers".
	ResourceType string
	// Kind is the recipe kind, e.g. "bicep".
	Kind string
	// Image is the recipe's OCI repository without a tag, e.g.
	// "ghcr.io/radius-project/kube-recipes/containers". Callers add the tag for
	// the channel they build for.
	Image string
	// Parameters are the recipe parameters declared by the pack, or nil.
	Parameters map[string]any
}

// kubernetesRecipes is the embedded Kubernetes recipe pack sorted by resource
// type. Populated once at package init and read-only afterwards.
var kubernetesRecipes []RecipePackRecipe

// loadKubernetesRecipePack populates the Kubernetes recipe pack from the
// embedded JSON. It is called once from the package init; see that function for
// the concurrency contract.
func loadKubernetesRecipePack(logger *log.Logger) {
	raw, err := manifestassets.FS.ReadFile(manifestassets.KubernetesRecipePackPath)
	if err != nil {
		logger.Printf("read %s: %s; default Kubernetes recipe pack disabled", manifestassets.KubernetesRecipePackPath, err)
		return
	}

	recipes, err := parseRecipePack(raw)
	if err != nil {
		logger.Printf("parse %s: %s; default Kubernetes recipe pack disabled", manifestassets.KubernetesRecipePackPath, err)
		return
	}
	kubernetesRecipes = recipes
}

// DefaultKubernetesRecipes returns the recipes of the Kubernetes recipe pack
// pinned in defaults.yaml, sorted by resource type. It returns nil when the
// embedded pack is missing or invalid; the cause is logged at init.
func DefaultKubernetesRecipes() []RecipePackRecipe {
	if kubernetesRecipes == nil {
		return nil
	}

	recipes := make([]RecipePackRecipe, len(kubernetesRecipes))
	for i, recipe := range kubernetesRecipes {
		recipe.Parameters = maps.Clone(recipe.Parameters)
		recipes[i] = recipe
	}
	return recipes
}

// recipePackTemplate is the subset of a compiled recipe pack template that the
// loader reads. Bicep emits symbolic-name resources (languageVersion 2.0) for
// templates that use extensions, so resources are keyed by symbolic name.
type recipePackTemplate struct {
	Resources map[string]struct {
		Type       string `json:"type"`
		Properties struct {
			Properties struct {
				Recipes map[string]struct {
					Kind       string         `json:"kind"`
					Source     string         `json:"source"`
					Parameters map[string]any `json:"parameters"`
				} `json:"recipes"`
			} `json:"properties"`
		} `json:"properties"`
	} `json:"resources"`
}

// parseRecipePack reads the recipes of the only Radius.Core/recipePacks
// resource in a compiled recipe pack template. Values must be literals: ARM
// expressions are not evaluated.
func parseRecipePack(raw []byte) ([]RecipePackRecipe, error) {
	var template recipePackTemplate
	if err := json.Unmarshal(raw, &template); err != nil {
		return nil, err
	}

	var packs []string
	for name, resource := range template.Resources {
		if strings.HasPrefix(resource.Type, recipePackResourceTypePrefix) {
			packs = append(packs, name)
		}
	}
	if len(packs) != 1 {
		return nil, fmt.Errorf("expected exactly one %s resource, found %d", strings.TrimSuffix(recipePackResourceTypePrefix, "@"), len(packs))
	}

	recipes := template.Resources[packs[0]].Properties.Properties.Recipes
	if len(recipes) == 0 {
		return nil, errors.New("recipe pack has no recipes")
	}

	result := make([]RecipePackRecipe, 0, len(recipes))
	for resourceType, recipe := range recipes {
		if recipe.Kind == "" {
			return nil, fmt.Errorf("recipe for %s has no kind", resourceType)
		}
		image, err := untaggedImage(recipe.Source)
		if err != nil {
			return nil, fmt.Errorf("recipe for %s: %w", resourceType, err)
		}
		for name, value := range recipe.Parameters {
			if s, ok := value.(string); ok && isExpression(s) {
				return nil, fmt.Errorf("recipe for %s: parameter %q is an expression", resourceType, name)
			}
		}
		result = append(result, RecipePackRecipe{
			ResourceType: resourceType,
			Kind:         recipe.Kind,
			Image:        image,
			Parameters:   recipe.Parameters,
		})
	}

	slices.SortFunc(result, func(a, b RecipePackRecipe) int {
		return strings.Compare(a.ResourceType, b.ResourceType)
	})
	return result, nil
}

// untaggedImage returns source without its tag. source must be a literal OCI
// reference with a tag and no digest.
func untaggedImage(source string) (string, error) {
	switch {
	case source == "":
		return "", errors.New("source is empty")
	case isExpression(source):
		return "", fmt.Errorf("source %q is an expression", source)
	case strings.Contains(source, "@"):
		return "", fmt.Errorf("source %q must use a tag, not a digest", source)
	}

	nameStart := strings.LastIndex(source, "/") + 1
	tagStart := strings.LastIndex(source[nameStart:], ":")
	if tagStart <= 0 {
		return "", fmt.Errorf("source %q has no tag", source)
	}
	return source[:nameStart+tagStart], nil
}

// isExpression reports whether s is an ARM template expression.
func isExpression(s string) bool {
	return strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "[[")
}
