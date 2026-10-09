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
	"slices"
	"strings"

	manifestassets "github.com/radius-project/radius/deploy/manifest"
	corerpv20250801 "github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
)

// recipePackResourceType is the resource type and API version the embedded
// pack must use. The pack is decoded with the models for this API version, so a
// pack written for another version fails to load instead of losing fields.
const recipePackResourceType = "Radius.Core/recipePacks@2025-08-01-preview"

var (
	// kubernetesRecipePackJSON is the validated properties object of the
	// embedded Kubernetes recipe pack. Set once at package init; each caller
	// decodes its own copy.
	kubernetesRecipePackJSON []byte
	// errKubernetesRecipePack is the reason the embedded pack failed to load.
	errKubernetesRecipePack = errors.New("default Kubernetes recipe pack not loaded")
)

// loadKubernetesRecipePack validates the embedded Kubernetes recipe pack. It is
// called once from the package init; see that function for the concurrency
// contract.
func loadKubernetesRecipePack(logger *log.Logger) {
	raw, err := manifestassets.FS.ReadFile(manifestassets.KubernetesRecipePackPath)
	if err == nil {
		kubernetesRecipePackJSON, err = parseRecipePack(raw)
	}
	if err != nil {
		errKubernetesRecipePack = fmt.Errorf("load %s: %w", manifestassets.KubernetesRecipePackPath, err)
		logger.Printf("%s; default Kubernetes recipe pack disabled", errKubernetesRecipePack)
		return
	}
	errKubernetesRecipePack = nil
}

// DefaultKubernetesRecipePack returns the properties of the Kubernetes recipe
// pack pinned in defaults.yaml, as authored upstream. Every call returns a new
// copy. It returns an error when the embedded pack is missing or invalid.
func DefaultKubernetesRecipePack() (*corerpv20250801.RecipePackProperties, error) {
	if errKubernetesRecipePack != nil {
		return nil, errKubernetesRecipePack
	}
	properties := &corerpv20250801.RecipePackProperties{}
	if err := json.Unmarshal(kubernetesRecipePackJSON, properties); err != nil {
		return nil, err
	}
	return properties, nil
}

// RecipeSourceRepository returns source without its tag. source must be a
// literal OCI reference with a tag and no digest.
func RecipeSourceRepository(source string) (string, error) {
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

// recipePackTemplate is the subset of a compiled recipe pack template that the
// loader reads. Bicep emits symbolic-name resources (languageVersion 2.0) for
// templates that use extensions, so resources are keyed by symbolic name.
// Everything else in the template, such as extension imports, is ignored.
type recipePackTemplate struct {
	Resources map[string]struct {
		Type       string `json:"type"`
		Properties struct {
			Properties json.RawMessage `json:"properties"`
		} `json:"properties"`
	} `json:"resources"`
}

// parseRecipePack validates the only Radius.Core/recipePacks resource in a
// compiled recipe pack template and returns its properties object. The
// properties must decode into the API models with no unknown or read-only
// fields, every recipe must have a supported kind and a tagged source, and
// every value must be a literal: ARM expressions are not evaluated.
func parseRecipePack(raw []byte) ([]byte, error) {
	var template recipePackTemplate
	if err := json.Unmarshal(raw, &template); err != nil {
		return nil, err
	}

	var packs []string
	for name, resource := range template.Resources {
		if strings.HasPrefix(resource.Type, "Radius.Core/recipePacks@") {
			packs = append(packs, name)
		}
	}
	if len(packs) != 1 {
		return nil, fmt.Errorf("expected exactly one Radius.Core/recipePacks resource, found %d", len(packs))
	}
	pack := template.Resources[packs[0]]
	if pack.Type != recipePackResourceType {
		return nil, fmt.Errorf("recipe pack type is %s, rad supports %s", pack.Type, recipePackResourceType)
	}
	if len(pack.Properties.Properties) == 0 {
		return nil, errors.New("recipe pack has no properties")
	}

	var properties corerpv20250801.RecipePackProperties
	if err := decodeKnownFields(pack.Properties.Properties, &properties); err != nil {
		return nil, fmt.Errorf("recipe pack: %w", err)
	}
	if properties.ProvisioningState != nil || properties.ReferencedBy != nil {
		return nil, errors.New("recipe pack sets read-only properties")
	}
	if len(properties.Recipes) == 0 {
		return nil, errors.New("recipe pack has no recipes")
	}

	var rawRecipes struct {
		Recipes map[string]json.RawMessage `json:"recipes"`
	}
	if err := json.Unmarshal(pack.Properties.Properties, &rawRecipes); err != nil {
		return nil, err
	}
	for resourceType, rawRecipe := range rawRecipes.Recipes {
		if err := validateRecipe(rawRecipe); err != nil {
			return nil, fmt.Errorf("recipe for %s: %w", resourceType, err)
		}
	}

	return json.Marshal(&properties)
}

// validateRecipe checks one recipe definition.
func validateRecipe(raw json.RawMessage) error {
	var recipe corerpv20250801.RecipeDefinition
	if err := decodeKnownFields(raw, &recipe); err != nil {
		return err
	}
	if recipe.Kind == nil || *recipe.Kind == "" {
		return errors.New("kind is empty")
	}
	if !slices.Contains(corerpv20250801.PossibleRecipeKindValues(), *recipe.Kind) {
		return fmt.Errorf("kind %q is not supported", *recipe.Kind)
	}
	if recipe.Source == nil {
		return errors.New("source is empty")
	}
	if _, err := RecipeSourceRepository(*recipe.Source); err != nil {
		return err
	}

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if path, ok := findExpression(value, ""); ok {
		return fmt.Errorf("%s is an expression", path)
	}
	return nil
}

// decodeKnownFields decodes raw into out and fails when raw has a top-level
// field that out does not keep. The generated models ignore unknown fields, so
// the check re-encodes out and compares field names.
func decodeKnownFields[T any](raw json.RawMessage, out *T) error {
	if err := json.Unmarshal(raw, out); err != nil {
		return err
	}

	var input, kept map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(encoded, &kept); err != nil {
		return err
	}

	var unknown []string
	for name, value := range input {
		if _, ok := kept[name]; !ok && value != nil {
			unknown = append(unknown, fmt.Sprintf("%q", name))
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)
		return fmt.Errorf("unsupported fields %s", strings.Join(unknown, ", "))
	}
	return nil
}

// findExpression returns the path of the first string in value that is an ARM
// template expression.
func findExpression(value any, path string) (string, bool) {
	switch v := value.(type) {
	case string:
		return path, isExpression(v)
	case map[string]any:
		names := make([]string, 0, len(v))
		for name := range v {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			if p, ok := findExpression(v[name], strings.TrimPrefix(path+"."+name, ".")); ok {
				return p, true
			}
		}
	case []any:
		for i, item := range v {
			if p, ok := findExpression(item, fmt.Sprintf("%s[%d]", path, i)); ok {
				return p, true
			}
		}
	}
	return "", false
}

// isExpression reports whether s is an ARM template expression.
func isExpression(s string) bool {
	return strings.HasPrefix(s, "[") && !strings.HasPrefix(s, "[[")
}
