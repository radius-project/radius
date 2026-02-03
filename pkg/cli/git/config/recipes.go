// ------------------------------------------------------------
// Copyright 2023 The Radius Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// ------------------------------------------------------------

package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/radius-project/radius/pkg/cli/git"
	"gopkg.in/yaml.v3"
)

// RecipeFile represents a .radius/config/recipes/*.yaml file.
type RecipeFile struct {
	// Name is the file identifier (e.g., "aws-default", "custom").
	Name string `yaml:"name"`

	// Recipes contains the recipe definitions in this file.
	Recipes []Recipe `yaml:"recipes"`
}

// Recipe maps a resource type to its deployment artifact.
type Recipe struct {
	// ResourceType identifies the Radius resource type.
	// Format: <Namespace>/<TypeName> (e.g., "Applications.Datastores/redisCaches")
	ResourceType string `yaml:"resourceType"`

	// RecipeKind specifies the IaC tool for deployment.
	// Valid values: "terraform", "bicep"
	RecipeKind string `yaml:"recipeKind"`

	// RecipeLocation is the OCI ref, git URL, or file path to the recipe module.
	RecipeLocation string `yaml:"recipeLocation"`
}

// RecipeKind constants for deployment tools.
const (
	RecipeKindTerraform = "terraform"
	RecipeKindBicep     = "bicep"
)

// resourceTypePattern validates resource type format.
var resourceTypePattern = regexp.MustCompile(`^[A-Za-z]+\.[A-Za-z]+/[a-zA-Z]+$`)

// gitRefPattern checks for git URLs with ref parameter.
var gitRefPattern = regexp.MustCompile(`\?ref=[^&]+`)

// ociLatestPattern checks for OCI references with latest tag.
var ociLatestPattern = regexp.MustCompile(`:latest$`)

// LoadRecipeFile loads and parses a single recipe YAML file.
func LoadRecipeFile(filePath string) (*RecipeFile, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, git.NewValidationError("failed to read recipe file", err.Error())
	}

	var recipeFile RecipeFile
	if err := yaml.Unmarshal(content, &recipeFile); err != nil {
		return nil, git.NewValidationError("failed to parse recipe file", err.Error())
	}

	return &recipeFile, nil
}

// LoadRecipes loads multiple recipe files and merges them into a single map.
// Later files in the list override earlier ones for the same resource type.
func LoadRecipes(filePaths []string) (map[string]Recipe, error) {
	recipes := make(map[string]Recipe)

	for _, filePath := range filePaths {
		recipeFile, err := LoadRecipeFile(filePath)
		if err != nil {
			return nil, err
		}

		for _, recipe := range recipeFile.Recipes {
			// Later files override earlier ones (last one wins)
			recipes[recipe.ResourceType] = recipe
		}
	}

	return recipes, nil
}

// Validate checks that the recipe file is well-formed.
func (rf *RecipeFile) Validate() error {
	var errors []string

	if rf.Name == "" {
		errors = append(errors, "recipe file name is required")
	}

	if len(rf.Recipes) == 0 {
		errors = append(errors, "recipe file must contain at least one recipe")
	}

	for i, recipe := range rf.Recipes {
		if err := recipe.Validate(); err != nil {
			errors = append(errors, fmt.Sprintf("recipe[%d]: %s", i, err.Error()))
		}
	}

	if len(errors) > 0 {
		return git.NewValidationError(
			fmt.Sprintf("recipe file '%s' validation failed", rf.Name),
			strings.Join(errors, "; "),
		)
	}

	return nil
}

// Validate checks that a single recipe definition is valid.
func (r *Recipe) Validate() error {
	var errors []string

	// Validate resource type format
	if r.ResourceType == "" {
		errors = append(errors, "resourceType is required")
	} else if !resourceTypePattern.MatchString(r.ResourceType) {
		errors = append(errors, fmt.Sprintf("invalid resourceType format: %s", r.ResourceType))
	}

	// Validate recipe kind
	switch r.RecipeKind {
	case RecipeKindTerraform, RecipeKindBicep:
		// Valid
	case "":
		errors = append(errors, "recipeKind is required")
	default:
		errors = append(errors, fmt.Sprintf("invalid recipeKind: %s (must be 'terraform' or 'bicep')", r.RecipeKind))
	}

	// Validate recipe location
	if r.RecipeLocation == "" {
		errors = append(errors, "recipeLocation is required")
	}

	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}

	return nil
}

// ValidatePinned checks that the recipe location is pinned to a specific version.
// Returns an error if the recipe is unpinned (e.g., using 'latest' or no git ref).
func (r *Recipe) ValidatePinned() error {
	loc := r.RecipeLocation

	// Git URLs must have a ref parameter
	if strings.HasPrefix(loc, "git::") || strings.Contains(loc, ".git//") {
		if !gitRefPattern.MatchString(loc) {
			return fmt.Errorf("git recipe '%s' must be pinned with ?ref=<version>", r.ResourceType)
		}
		return nil
	}

	// OCI references must not use 'latest' tag
	if strings.HasPrefix(loc, "br:") || strings.HasPrefix(loc, "oci://") {
		if ociLatestPattern.MatchString(loc) {
			return fmt.Errorf("OCI recipe '%s' must not use ':latest' tag", r.ResourceType)
		}
		return nil
	}

	// File paths are considered pinned (local development)
	if strings.HasPrefix(loc, "./") || strings.HasPrefix(loc, "/") {
		return nil
	}

	// For other formats, we can't determine if it's pinned
	return nil
}

// IsTerraform returns true if the recipe uses Terraform.
func (r *Recipe) IsTerraform() bool {
	return r.RecipeKind == RecipeKindTerraform
}

// IsBicep returns true if the recipe uses Bicep.
func (r *Recipe) IsBicep() bool {
	return r.RecipeKind == RecipeKindBicep
}

// ExtractVersion attempts to extract the version from the recipe location.
// Returns the version string and true if found, empty string and false otherwise.
func (r *Recipe) ExtractVersion() (string, bool) {
	loc := r.RecipeLocation

	// Git URL: extract ref parameter
	if gitRefPattern.MatchString(loc) {
		matches := regexp.MustCompile(`\?ref=([^&]+)`).FindStringSubmatch(loc)
		if len(matches) > 1 {
			return matches[1], true
		}
	}

	// OCI reference: extract tag after last colon
	if strings.HasPrefix(loc, "br:") || strings.HasPrefix(loc, "oci://") {
		parts := strings.Split(loc, ":")
		if len(parts) >= 2 {
			tag := parts[len(parts)-1]
			if tag != "" && tag != "latest" {
				return tag, true
			}
		}
	}

	return "", false
}

// LookupRecipe finds a recipe for the given resource type.
func LookupRecipe(recipes map[string]Recipe, resourceType string) (*Recipe, bool) {
	recipe, found := recipes[resourceType]
	if !found {
		return nil, false
	}
	return &recipe, true
}
