/*
Copyright 2026 The Radius Authors.

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

package deployment

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
	"github.com/radius-project/radius/pkg/schema"
	"github.com/radius-project/radius/pkg/ucp/resources"
)

var symbolicIDReference = regexp.MustCompile(`^\[reference\('([^']+)'\)\.id\]$`)

// PlanDelegatedResources places evaluated delegate inputs on their target's existing native PUT.
// The engine still evaluates expressions and publishes ordinary logical resource references.
// Definitions and schemas use symbolic paths such as "child/route" for embedded module resources.
// Copy loops are engine-evaluated; indexed cross-module targets and unresolved identities are rejected.
func PlanDelegatedResources(template map[string]any, scope string, definitions map[string]*recipes.EnvironmentDefinition, schemas map[string]map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(template)
	if err != nil {
		return nil, err
	}
	var planned map[string]any
	if err := json.Unmarshal(encoded, &planned); err != nil {
		return nil, err
	}
	changed, err := planDelegationScope(planned, scope, "", definitions, schemas)
	if err != nil {
		return nil, err
	}
	if !changed {
		return template, nil
	}
	return planned, nil
}

func planDelegationScope(planned map[string]any, scope, path string, allDefinitions map[string]*recipes.EnvironmentDefinition, allSchemas map[string]map[string]any) (bool, error) {
	definitions := map[string]*recipes.EnvironmentDefinition{}
	schemas := map[string]map[string]any{}
	declarations, _ := planned["resources"].(map[string]any)
	for symbol := range declarations {
		definitions[symbol] = allDefinitions[path+symbol]
		schemas[symbol] = allSchemas[path+symbol]
	}
	moduleInputs, changed, err := planDelegationModules(planned, scope, path, allDefinitions, allSchemas)
	if err != nil {
		return false, err
	}
	groups := map[string][]string{}
	targetIndexes := map[string]string{}
	symbols := make([]string, 0, len(declarations))
	for symbol := range declarations {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		declaration, _ := declarations[symbol].(map[string]any)
		definition := definitions[symbol]
		if definition == nil || definition.Driver != "delegated" || declaration["existing"] == true {
			continue
		}
		body, _ := declaration["properties"].(map[string]any)
		if body == nil {
			return false, fmt.Errorf("delegating resource %q requires native resource properties", symbol)
		}
		properties, _ := body["properties"].(map[string]any)
		values := annotatedValues(properties, schemas[symbol], definition.DelegatesTo)
		target := ""
		for _, value := range values {
			id, ok := value.(string)
			if !ok {
				return false, fmt.Errorf("delegation reference for %q must be a string", symbol)
			}
			selected, index := "", ""
			if strings.HasPrefix(id, "[") && !strings.HasPrefix(id, "[[") && strings.HasSuffix(id, "]") {
				var identity bool
				selected, index, identity = delegationSymbolReference(id[1 : len(id)-1])
				if !identity {
					selected = ""
				}
			}
			if selected == "" {
				for _, candidate := range symbols {
					other, _ := declarations[candidate].(map[string]any)
					otherBody, _ := other["properties"].(map[string]any)
					name, _ := otherBody["name"].(string)
					typeName, _, _ := strings.Cut(fmt.Sprint(other["type"]), "@")
					if name != "" && !strings.HasPrefix(name, "[") && strings.EqualFold(scope+"/providers/"+typeName+"/"+name, id) {
						selected = candidate
						break
					}
				}
			}
			if selected == "" {
				return false, fmt.Errorf("cannot identify the declared delegation target for %q from compiled identity metadata", symbol)
			}
			if target != "" && selected != target {
				return false, fmt.Errorf("delegation requires exactly one distinct target resource")
			}
			if target != "" && targetIndexes[symbol] != index {
				return false, fmt.Errorf("delegation resource %q requires exactly one distinct target instance expression", symbol)
			}
			other, _ := declarations[selected].(map[string]any)
			if other["copy"] != nil && index == "" {
				return false, fmt.Errorf("copied delegation target %q requires an instance index", selected)
			}
			if other["copy"] == nil && index != "" {
				return false, fmt.Errorf("delegation target %q is not a copied resource", selected)
			}
			propertyLambda := false
			computedSelection := false
			rewriteARM(index, func(call string) string {
				propertyLambda = propertyLambda || strings.HasPrefix(call, "lambdaVariables(")
				computedSelection = computedSelection || strings.HasPrefix(call, "reference(") || strings.HasPrefix(call, "references(")
				return call
			})
			if propertyLambda {
				return false, fmt.Errorf("delegation target instance for %q must use the resource copy scope, not a property lambda", symbol)
			}
			if computedSelection {
				return false, fmt.Errorf("delegation target instance for %q cannot depend on computed resource outputs", symbol)
			}
			targetIndexes[symbol] = index
			target = selected
		}
		if target == "" {
			return false, fmt.Errorf("delegation requires a typed target reference for %q", symbol)
		}
		targetDeclaration, _ := declarations[target].(map[string]any)
		targetType, _, _ := strings.Cut(fmt.Sprint(targetDeclaration["type"]), "@")
		if !strings.EqualFold(targetType, definition.DelegatesTo) {
			return false, fmt.Errorf("declared delegation target for %q has the wrong resource type", symbol)
		}
		if targetDeclaration["existing"] == true {
			// Existing targets are reconciled by the ordinary bound-resource update path.
			continue
		}
		targetDefinition := definitions[target]
		sourceType, _, _ := strings.Cut(fmt.Sprint(declaration["type"]), "@")
		if targetDefinition == nil || !slices.ContainsFunc(targetDefinition.AcceptsDelegationFrom, func(accepted string) bool {
			return strings.EqualFold(accepted, sourceType)
		}) {
			return false, fmt.Errorf("declared target does not accept delegation from %q", sourceType)
		}
		groups[target] = append(groups[target], symbol)
	}
	for _, target := range symbols {
		definition := definitions[target]
		declaration, _ := declarations[target].(map[string]any)
		if definition == nil || len(definition.AcceptsDelegationFrom) == 0 || declaration["existing"] == true {
			continue
		}
		targetCopy, _ := declaration["copy"].(map[string]any)
		targetCopyName, _ := targetCopy["name"].(string)
		if targetCopy != nil && (targetCopyName == "" || targetCopy["count"] == nil) {
			return false, fmt.Errorf("delegation target loop %q requires a name and count", target)
		}
		body, _ := declaration["properties"].(map[string]any)
		if body == nil {
			return false, fmt.Errorf("delegation target %q requires native resource properties", target)
		}
		targetType, _, _ := strings.Cut(fmt.Sprint(declaration["type"]), "@")
		targetID, err := declarationID(scope, targetType, body["name"])
		if err != nil {
			return false, err
		}
		delegates := groups[target]
		dependencies := stringValues(declaration["dependsOn"])
		parts := []string{"createArray()"}
		for _, input := range moduleInputs[target] {
			parts = append(parts, input.expression)
			if input.symbol != "" {
				dependencies = append(dependencies, input.symbol)
			}
		}
		for _, source := range delegates {
			sourceDeclaration := declarations[source].(map[string]any)
			sourceBody, _ := sourceDeclaration["properties"].(map[string]any)
			sourceType, version, _ := strings.Cut(fmt.Sprint(sourceDeclaration["type"]), "@")
			sourceID, err := declarationID(scope, sourceType, sourceBody["name"])
			if err != nil {
				return false, err
			}
			member := map[string]any{"id": sourceID, "type": sourceType, "apiVersion": version, "properties": sourceBody["properties"]}
			for _, name := range []string{"location", "tags"} {
				if value, present := sourceBody[name]; present {
					member[name] = value
				}
			}
			if targetCopy != nil {
				replaced, err := replaceIndexedTargetIdentity(member, target, targetID, targetCopyName)
				if err != nil {
					return false, err
				}
				member = replaced.(map[string]any)
			} else {
				member = replaceIdentityReference(member, target, targetID).(map[string]any)
			}
			if delegationReferences(member, target) {
				return false, fmt.Errorf("desired delegation input for %q depends on computed target output %q", source, target)
			}
			memberExpression, err := delegationMembersWithTarget(sourceDeclaration, member, source, targetIndexes[source], targetCopyName)
			if err != nil {
				return false, err
			}
			parts = append(parts, memberExpression)
			dependencies = append(dependencies, stringValues(sourceDeclaration["dependsOn"])...)
			sourceName := sourceBody["name"]
			if targetCopy != nil {
				sourceName, err = indexedDelegationSourceName(sourceDeclaration, declaration, targetIndexes[source])
				if err != nil {
					return false, err
				}
			}
			sourceDeclaration["existing"] = true
			sourceDeclaration["properties"] = map[string]any{"name": sourceName}
			sourceDeclaration["dependsOn"] = []string{target}
		}
		filtered := make([]string, 0, len(dependencies))
		for _, dependency := range dependencies {
			dependency = delegationDependencySymbol(dependency, declarations)
			if dependency != target && !slices.Contains(delegates, dependency) && !slices.Contains(filtered, dependency) {
				filtered = append(filtered, dependency)
			}
		}
		slices.Sort(filtered)
		declaration["dependsOn"] = filtered
		properties, _ := body["properties"].(map[string]any)
		if properties == nil {
			return false, fmt.Errorf("delegation target %q requires desired properties", target)
		}
		expression := "concat(" + strings.Join(parts, ",") + ")"
		properties["__radiusDelegatingResources"] = "[" + expression + "]"
		changed = true
	}
	for _, target := range symbols {
		definition := definitions[target]
		declaration, _ := declarations[target].(map[string]any)
		if definition == nil || len(definition.AcceptsDelegationFrom) == 0 || declaration["existing"] == true {
			continue
		}
		visited := map[string]bool{}
		var reachesTarget func(string) bool
		reachesTarget = func(symbol string) bool {
			if symbol == target {
				return true
			}
			if visited[symbol] {
				return false
			}
			visited[symbol] = true
			resource, _ := declarations[symbol].(map[string]any)
			for _, dependency := range stringValues(resource["dependsOn"]) {
				if reachesTarget(dependency) {
					return true
				}
			}
			return false
		}
		for _, dependency := range stringValues(declaration["dependsOn"]) {
			if reachesTarget(dependency) {
				return false, fmt.Errorf("delegation target %q has a cyclic desired-input prerequisite %q", target, dependency)
			}
		}
	}
	return changed, nil
}

// armObjectValue emits constructors, leaving all expression evaluation to the deployment engine.
func armObjectValue(value any) (string, error) {
	switch value := value.(type) {
	case nil:
		return "json('null')", nil
	case string:
		if strings.HasPrefix(value, "[") && !strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]") {
			return strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"), nil
		}
		if strings.HasPrefix(value, "[[") {
			value = value[1:]
		}
		return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
	case bool:
		return fmt.Sprintf("bool('%t')", value), nil
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), nil
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			expression, err := armObjectValue(item)
			if err != nil {
				return "", err
			}
			items = append(items, expression)
		}
		return "createArray(" + strings.Join(items, ",") + ")", nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		items := make([]string, 0, len(keys)*2)
		for _, key := range keys {
			copies, isCopy := value[key].([]any)
			if key == "copy" && isCopy {
				for _, raw := range copies {
					copy, _ := raw.(map[string]any)
					name, _ := copy["name"].(string)
					if name == "" || copy["count"] == nil {
						return "", fmt.Errorf("compiled delegation property copy requires a name and count")
					}
					count, err := armObjectValue(copy["count"])
					if err != nil {
						return "", err
					}
					input, err := armObjectValue(copy["input"])
					if err != nil {
						return "", err
					}
					index := "__radiusPropertyIndex"
					for strings.Contains(input, "'"+index+"'") {
						index += "_"
					}
					input, err = delegationCopyIndex(input, name, index, false)
					if err != nil {
						return "", err
					}
					items = append(items, "'"+strings.ReplaceAll(name, "'", "''")+"'",
						fmt.Sprintf("map(range(0,%s),lambda('%s',%s))", count, index, input))
				}
				continue
			}
			expression, err := armObjectValue(value[key])
			if err != nil {
				return "", err
			}
			items = append(items, "'"+strings.ReplaceAll(key, "'", "''")+"'", expression)
		}
		return "createObject(" + strings.Join(items, ",") + ")", nil
	default:
		return "", fmt.Errorf("unsupported compiled delegation value %T", value)
	}
}

func declarationID(scope, resourceType string, name any) (string, error) {
	text, ok := name.(string)
	if !ok || text == "" {
		return "", fmt.Errorf("delegation declaration requires a resource name")
	}
	if !strings.HasPrefix(text, "[") {
		id, err := resources.ParseResource(scope + "/providers/" + resourceType + "/" + text)
		if err != nil {
			return "", err
		}
		return id.String(), nil
	}
	return fmt.Sprintf("[format('%s/providers/%s/{0}',%s)]", strings.ReplaceAll(scope, "'", "''"), resourceType,
		strings.TrimSuffix(strings.TrimPrefix(text, "["), "]")), nil
}

func replaceIdentityReference(value any, symbol, replacement string) any {
	switch value := value.(type) {
	case string:
		if !strings.HasPrefix(value, "[") || strings.HasPrefix(value, "[[") || !strings.HasSuffix(value, "]") {
			return value
		}
		pattern := "reference('" + symbol + "').id"
		expression := replacement
		if strings.HasPrefix(expression, "[") {
			expression = strings.TrimSuffix(strings.TrimPrefix(expression, "["), "]")
		} else {
			expression = "'" + strings.ReplaceAll(expression, "'", "''") + "'"
		}
		return "[" + rewriteARM(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"), func(call string) string {
			if call == pattern && !strings.HasPrefix(expression, "[") {
				return expression
			}
			return call
		}) + "]"
	case map[string]any:
		for key, child := range value {
			value[key] = replaceIdentityReference(child, symbol, replacement)
		}
	case []any:
		for index, child := range value {
			value[index] = replaceIdentityReference(child, symbol, replacement)
		}
	}
	return value
}

func annotatedValues(value any, field map[string]any, targetType string) []any {
	if readOnly, _ := field["readOnly"].(bool); readOnly {
		return nil
	}
	if reference, _ := field[schema.ResourceReferenceAnnotation].(string); strings.EqualFold(reference, targetType) {
		return []any{value}
	}
	result := []any{}
	switch value := value.(type) {
	case string:
		if strings.HasPrefix(value, "[") && !strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]") {
			return delegationAnnotatedExpression(value[1:len(value)-1], field, targetType)
		}
	case map[string]any:
		properties, _ := field["properties"].(map[string]any)
		for key, child := range value {
			if key == "copy" {
				copies, _ := child.([]any)
				for _, raw := range copies {
					copy, _ := raw.(map[string]any)
					name, _ := copy["name"].(string)
					property, _ := properties[name].(map[string]any)
					items, _ := property["items"].(map[string]any)
					result = append(result, annotatedValues(copy["input"], items, targetType)...)
				}
				continue
			}
			raw, exists := properties[key]
			if !exists {
				raw = field["additionalProperties"]
			}
			childSchema, _ := raw.(map[string]any)
			result = append(result, annotatedValues(child, childSchema, targetType)...)
		}
	case []any:
		items, _ := field["items"].(map[string]any)
		for _, child := range value {
			result = append(result, annotatedValues(child, items, targetType)...)
		}
	}
	return result
}

func stringValues(value any) []string {
	if values, ok := value.([]string); ok {
		return slices.Clone(values)
	}
	values, _ := value.([]any)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}
