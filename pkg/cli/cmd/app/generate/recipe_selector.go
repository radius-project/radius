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

package generate

import (
	"context"
	"fmt"
	"sort"

	"github.com/radius-project/radius/pkg/cli/prompt"
	"github.com/radius-project/radius/pkg/discovery"
	"github.com/radius-project/radius/pkg/discovery/config"
	"github.com/radius-project/radius/pkg/discovery/recipes"
)

// RecipeSelector handles interactive recipe selection for dependencies.
type RecipeSelector struct {
	prompter      prompt.Interface
	recipeProfile string
	verbose       bool
}

// NewRecipeSelector creates a new RecipeSelector.
func NewRecipeSelector(prompter prompt.Interface, recipeProfile string, verbose bool) *RecipeSelector {
	return &RecipeSelector{
		prompter:      prompter,
		recipeProfile: recipeProfile,
		verbose:       verbose,
	}
}

// RecipeSelection represents a user's recipe selection for a dependency.
type RecipeSelection struct {
	DependencyID string
	Recipe       *recipes.Recipe
	Skipped      bool
}

// SelectRecipesForDependencies prompts the user to select recipes for each dependency.
func (s *RecipeSelector) SelectRecipesForDependencies(
	ctx context.Context,
	dependencies []discovery.DetectedDependency,
	resourceTypes []discovery.ResourceTypeMapping,
) ([]RecipeSelection, error) {
	// Load recipe source configuration
	cfg, err := config.LoadSourcesConfig()
	if err != nil {
		// No config found, return empty selections
		return nil, nil
	}

	// Get sources for the current profile
	sources := cfg.GetSourcesForProfile(s.recipeProfile)
	if len(sources) == 0 {
		return nil, nil
	}

	// Create recipe registry
	registry := recipes.NewRegistry()
	for _, srcCfg := range sources {
		// Resolve auth
		var auth *recipes.SourceCredentials
		if srcCfg.Auth != nil {
			resolved, err := srcCfg.Auth.ResolveAuth()
			if err != nil {
				continue // Skip sources with auth errors
			}
			if resolved != nil && resolved.HasCredentials() {
				auth = &recipes.SourceCredentials{
					Token:    resolved.Token,
					Username: resolved.Username,
					Password: resolved.Password,
				}
			}
		}

		source, err := recipes.CreateSource(recipes.SourceConfig{
			Name:        srcCfg.Name,
			Type:        srcCfg.Type,
			URL:         srcCfg.URL,
			Credentials: auth,
		})
		if err != nil {
			continue
		}
		_ = registry.Register(source)
	}

	// Match recipes for each resource type
	recipeMatches, err := registry.MatchRecipes(ctx, resourceTypes)
	if err != nil {
		return nil, fmt.Errorf("matching recipes: %w", err)
	}

	// Group recipes by dependency
	recipesByDep := make(map[string][]discovery.RecipeMatch)
	for _, match := range recipeMatches {
		recipesByDep[match.DependencyID] = append(recipesByDep[match.DependencyID], match)
	}

	// Prompt for each dependency with multiple recipes
	var selections []RecipeSelection
	for _, dep := range dependencies {
		matches := recipesByDep[dep.ID]
		if len(matches) == 0 {
			// No recipes found
			selections = append(selections, RecipeSelection{
				DependencyID: dep.ID,
				Skipped:      true,
			})
			continue
		}

		if len(matches) == 1 {
			// Single recipe, auto-select
			selections = append(selections, RecipeSelection{
				DependencyID: dep.ID,
				Recipe: &recipes.Recipe{
					Name:         matches[0].Recipe.Name,
					SourceType:   string(matches[0].Recipe.SourceType),
					TemplatePath: matches[0].Recipe.SourceLocation,
					Version:      matches[0].Recipe.Version,
					Description:  matches[0].Recipe.Description,
				},
			})
			continue
		}

		// Multiple recipes - prompt user
		selected, err := s.promptRecipeSelection(dep, matches)
		if err != nil {
			return nil, err
		}
		selections = append(selections, selected)
	}

	return selections, nil
}

func (s *RecipeSelector) promptRecipeSelection(
	dep discovery.DetectedDependency,
	matches []discovery.RecipeMatch,
) (RecipeSelection, error) {
	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	// Build option list
	options := make([]string, 0, len(matches)+1)
	for i, match := range matches {
		option := fmt.Sprintf("[%d] %s (%s) - %.0f%% match",
			i+1,
			match.Recipe.Name,
			match.Recipe.SourceType,
			match.Score*100,
		)
		if match.Recipe.Description != "" {
			option += fmt.Sprintf("\n    %s", match.Recipe.Description)
		}
		options = append(options, option)
	}
	options = append(options, "[S] Skip - no recipe")

	// Prompt
	promptMsg := fmt.Sprintf("Select recipe for %s (%s):", dep.Name, dep.Type)
	selected, err := s.prompter.GetListInput(options, promptMsg)
	if err != nil {
		return RecipeSelection{}, err
	}

	// Parse selection
	if selected == options[len(options)-1] {
		return RecipeSelection{
			DependencyID: dep.ID,
			Skipped:      true,
		}, nil
	}

	// Find selected recipe
	for i, opt := range options[:len(options)-1] {
		if opt == selected {
			match := matches[i]
			return RecipeSelection{
				DependencyID: dep.ID,
				Recipe: &recipes.Recipe{
					Name:         match.Recipe.Name,
					SourceType:   string(match.Recipe.SourceType),
					TemplatePath: match.Recipe.SourceLocation,
					Version:      match.Recipe.Version,
					Description:  match.Recipe.Description,
				},
			}, nil
		}
	}

	return RecipeSelection{DependencyID: dep.ID, Skipped: true}, nil
}

// ApplySelectionsToResult applies recipe selections to the discovery result.
func ApplySelectionsToResult(result *discovery.DiscoveryResult, selections []RecipeSelection) {
	if result == nil || len(selections) == 0 {
		return
	}

	// Clear existing recipe matches
	result.Recipes = nil

	for _, sel := range selections {
		if sel.Skipped || sel.Recipe == nil {
			continue
		}

		result.Recipes = append(result.Recipes, discovery.RecipeMatch{
			DependencyID: sel.DependencyID,
			Recipe: discovery.Recipe{
				Name:           sel.Recipe.Name,
				SourceType:     discovery.RecipeSourceType(sel.Recipe.SourceType),
				SourceLocation: sel.Recipe.TemplatePath,
				Version:        sel.Recipe.Version,
				Description:    sel.Recipe.Description,
			},
			Score:        1.0, // User-selected
			MatchReasons: []string{"user selected"},
		})
	}
}
