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

package show

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/radius-project/radius/pkg/corerp/api/v20250801preview"
	"sigs.k8s.io/yaml"
)

func (r *Runner) display(recipePack v20250801preview.RecipePackResource) error {
	if recipePack.Properties == nil || recipePack.Properties.Recipes == nil || len(recipePack.Properties.Recipes) == 0 {
		r.Output.LogInfo("\nRECIPES: none")
		return nil
	}

	r.Output.LogInfo("\nRECIPES:")

	resourceTypes := make([]string, 0, len(recipePack.Properties.Recipes))
	for resourceType := range recipePack.Properties.Recipes {
		resourceTypes = append(resourceTypes, resourceType)
	}
	sort.Strings(resourceTypes)

	for idx, resourceType := range resourceTypes {
		definition := recipePack.Properties.Recipes[resourceType]
		if definition == nil {
			continue
		}

		kind := "unknown"
		if definition.RecipeKind != nil {
			kind = string(*definition.RecipeKind)
		}

		location := ""
		if definition.RecipeLocation != nil {
			location = *definition.RecipeLocation
		}

		r.Output.LogInfo("%s", resourceType)
		r.Output.LogInfo("   Kind: %s", kind)
		r.Output.LogInfo("   Location: %s", location)

		if len(definition.Parameters) > 0 {
			formatted, err := formatRecipeParameters(definition.Parameters)
			if err != nil {
				return fmt.Errorf("format recipe parameters: %w", err)
			}
			if formatted != "" {
				r.Output.LogInfo("   Parameters:")
				r.Output.LogInfo("%s", indentLines(formatted, "      "))
			}
		}

		if idx < len(resourceTypes)-1 {
			r.Output.LogInfo("")
		}
	}

	return nil
}

// formatRecipeParameters renders the parameters map without JSON braces/quotes.
func formatRecipeParameters(params map[string]any) (string, error) {
	raw, err := json.Marshal(params)
	if err != nil {
		return "", err
	}

	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "", err
	}
	if len(normalized) == 0 {
		return "", nil
	}

	out, err := yaml.Marshal(normalized)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

// indentLines prefixes each line with the provided indent string.
func indentLines(text, indent string) string {
	if text == "" {
		return ""
	}

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}
