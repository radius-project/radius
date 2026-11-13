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
	"maps"
	"slices"
	"strings"

	"github.com/radius-project/radius/pkg/recipes"
)

type delegationModuleInput struct {
	symbol     string
	expression string
}

func delegationModuleTemplate(declaration map[string]any) map[string]any {
	typeName, _, _ := strings.Cut(fmt.Sprint(declaration["type"]), "@")
	if !strings.EqualFold(typeName, "Microsoft.Resources/deployments") {
		return nil
	}
	properties, _ := declaration["properties"].(map[string]any)
	template, _ := properties["template"].(map[string]any)
	return template
}

func cloneDelegationObject(value map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var copy map[string]any
	err = json.Unmarshal(encoded, &copy)
	return copy, err
}

// planDelegationModules separates desired-input evaluation from the original module's GETs and outputs.
// Only modules containing cross-boundary delegates are split; their parameter and variable scopes stay inner.
func planDelegationModules(template map[string]any, scope, path string, definitions map[string]*recipes.EnvironmentDefinition, schemas map[string]map[string]any) (map[string][]delegationModuleInput, bool, error) {
	inputs := map[string][]delegationModuleInput{}
	declarations, _ := template["resources"].(map[string]any)
	symbols := make([]string, 0, len(declarations))
	for symbol := range declarations {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	changed := false
	for _, moduleSymbol := range symbols {
		module, _ := declarations[moduleSymbol].(map[string]any)
		child := delegationModuleTemplate(module)
		if child == nil {
			continue
		}
		childResources, _ := child["resources"].(map[string]any)
		moduleProperties, _ := module["properties"].(map[string]any)
		arguments, _ := moduleProperties["parameters"].(map[string]any)
		groups := map[string][]string{}
		childDefinitions := definitions
		childSymbols := make([]string, 0, len(childResources))
		for source := range childResources {
			childSymbols = append(childSymbols, source)
		}
		slices.Sort(childSymbols)
		for _, source := range childSymbols {
			definition := definitions[path+moduleSymbol+"/"+source]
			declaration, _ := childResources[source].(map[string]any)
			if definition == nil || definition.Driver != "delegated" || declaration["existing"] == true {
				continue
			}
			body, _ := declaration["properties"].(map[string]any)
			values := annotatedValues(body["properties"], schemas[path+moduleSymbol+"/"+source], definition.DelegatesTo)
			target := ""
			for _, value := range values {
				resolved := delegationModuleIdentity(value, child, arguments, childResources, declarations, scope, definition.DelegatesTo)
				if resolved == "" {
					continue
				}
				if target != "" && target != resolved {
					return nil, false, fmt.Errorf("delegation requires exactly one distinct target resource")
				}
				target = resolved
			}
			if target == "" {
				continue
			}
			// All typed leaves must resolve to the same declared parent, not just one convenient leaf.
			for _, value := range values {
				if delegationModuleIdentity(value, child, arguments, childResources, declarations, scope, definition.DelegatesTo) != target {
					return nil, false, fmt.Errorf("cross-module delegate %q has an unresolved or different target", source)
				}
			}
			targetDeclaration, _ := declarations[target].(map[string]any)
			if targetDeclaration["existing"] == true {
				childDefinitions = maps.Clone(childDefinitions)
				delete(childDefinitions, path+moduleSymbol+"/"+source)
				continue
			}
			sourceType, _, _ := strings.Cut(fmt.Sprint(declaration["type"]), "@")
			targetDefinition := definitions[path+target]
			if targetDefinition == nil || !slices.ContainsFunc(targetDefinition.AcceptsDelegationFrom, func(accepted string) bool {
				return strings.EqualFold(accepted, sourceType)
			}) {
				return nil, false, fmt.Errorf("declared target does not accept delegation from %q", sourceType)
			}
			groups[target] = append(groups[target], source)
		}
		if len(groups) > 0 {
			if module["copy"] != nil || module["scope"] != nil || module["resourceGroup"] != nil || module["subscriptionId"] != nil {
				return nil, false, fmt.Errorf("cross-module delegation %q requires a single same-scope embedded module", moduleSymbol)
			}
			evaluation, _ := moduleProperties["expressionEvaluationOptions"].(map[string]any)
			if evaluation["scope"] != "inner" {
				return nil, false, fmt.Errorf("cross-module delegation %q requires inner expression evaluation", moduleSymbol)
			}
			inputSymbol := "__radiusInputs_" + moduleSymbol
			if _, exists := declarations[inputSymbol]; exists {
				return nil, false, fmt.Errorf("delegation input module symbol %q already exists", inputSymbol)
			}
			inputModule, err := cloneDelegationObject(module)
			if err != nil {
				return nil, false, err
			}
			inputTemplate := delegationModuleTemplate(inputModule)
			inputProperties := inputModule["properties"].(map[string]any)
			inputTemplate["resources"] = map[string]any{}
			inputTemplate["outputs"] = map[string]any{}
			prerequisites := []string{}
			name, err := armObjectValue(module["name"])
			if err != nil || module["name"] == nil {
				return nil, false, fmt.Errorf("delegation module %q requires a deployment name", moduleSymbol)
			}
			inputModule["name"] = "[concat(" + name + ",'-radius-inputs')]"
			targets := make([]string, 0, len(groups))
			for target := range groups {
				targets = append(targets, target)
			}
			slices.Sort(targets)
			for index, target := range targets {
				targetDeclaration := declarations[target].(map[string]any)
				if targetDeclaration["copy"] != nil {
					return nil, false, fmt.Errorf("cross-module delegation target %q uses an unsupported indexed target loop", target)
				}
				targetBody, _ := targetDeclaration["properties"].(map[string]any)
				if slices.Contains(stringValues(targetDeclaration["dependsOn"]), moduleSymbol) || delegationReferences(targetBody, moduleSymbol) {
					return nil, false, fmt.Errorf("target %q desired inputs depend on publication module %q", target, moduleSymbol)
				}
				targetType, _, _ := strings.Cut(fmt.Sprint(targetDeclaration["type"]), "@")
				targetID, err := declarationID(scope, targetType, targetBody["name"])
				if err != nil {
					return nil, false, err
				}
				inputProperties["parameters"] = replaceIdentityReference(inputProperties["parameters"], target, targetID)
				parameter := fmt.Sprintf("__radiusTarget%d", index)
				parameters, _ := inputTemplate["parameters"].(map[string]any)
				if parameters == nil {
					parameters = map[string]any{}
					inputTemplate["parameters"] = parameters
				}
				if _, exists := parameters[parameter]; exists {
					return nil, false, fmt.Errorf("delegation input parameter %q already exists", parameter)
				}
				parameters[parameter] = map[string]any{"type": "string"}
				inputArguments, _ := inputProperties["parameters"].(map[string]any)
				if inputArguments == nil {
					inputArguments = map[string]any{}
					inputProperties["parameters"] = inputArguments
				}
				inputArguments[parameter] = map[string]any{"value": targetID}
				parts := []string{"createArray()"}
				for _, source := range groups[target] {
					sourceDeclaration := childResources[source].(map[string]any)
					sourceBody, _ := sourceDeclaration["properties"].(map[string]any)
					sourceType, version, _ := strings.Cut(fmt.Sprint(sourceDeclaration["type"]), "@")
					sourceID, err := declarationID(scope, sourceType, sourceBody["name"])
					if err != nil {
						return nil, false, err
					}
					member := map[string]any{"id": sourceID, "type": sourceType, "apiVersion": version, "properties": sourceBody["properties"]}
					for _, key := range []string{"location", "tags"} {
						if value, exists := sourceBody[key]; exists {
							member[key] = value
						}
					}
					for _, candidate := range childSymbols {
						if delegationModuleIdentity("[reference('"+candidate+"').id]", child, arguments, childResources, declarations, scope, targetType) == target {
							member = replaceIdentityReference(member, candidate, "[parameters('"+parameter+"')]").(map[string]any)
						}
					}
					for _, dependency := range stringValues(sourceDeclaration["dependsOn"]) {
						if delegationModuleIdentity("[reference('"+dependency+"').id]", child, arguments, childResources, declarations, scope, targetType) != target {
							prerequisites = append(prerequisites, dependency)
						}
					}
					expression, err := delegationMembers(sourceDeclaration, member, source)
					if err != nil {
						return nil, false, err
					}
					parts = append(parts, expression)
					sourceDeclaration["existing"] = true
					sourceDeclaration["properties"] = map[string]any{"name": sourceBody["name"]}
					sourceDeclaration["dependsOn"] = []string{}
				}
				output := fmt.Sprintf("delegates%d", index)
				inputTemplate["outputs"].(map[string]any)[output] = map[string]any{"type": "array", "value": "[concat(" + strings.Join(parts, ",") + ")]"}
				expression := fmt.Sprintf("reference('%s').outputs.%s.value", inputSymbol, output)
				dependency := inputSymbol
				if condition, present := module["condition"]; present {
					conditionExpression, err := armObjectValue(condition)
					if err != nil {
						return nil, false, err
					}
					version, _ := module["apiVersion"].(string)
					if version == "" {
						_, version, _ = strings.Cut(fmt.Sprint(module["type"]), "@")
					}
					if version == "" {
						return nil, false, fmt.Errorf("conditional delegation module %q requires an API version", moduleSymbol)
					}
					expression = fmt.Sprintf("if(%s,reference('%s','%s').outputs.%s.value,createArray())", conditionExpression, inputSymbol, version, output)
					// The guarded reference supplies the native dependency only when this module is active.
					dependency = ""
				}
				inputs[target] = append(inputs[target], delegationModuleInput{symbol: dependency, expression: expression})
			}
			if err := delegationInputPrerequisites(inputTemplate, childResources, groups, prerequisites); err != nil {
				return nil, false, fmt.Errorf("module %q: %w", moduleSymbol, err)
			}
			pruneDelegationInputParameters(inputTemplate, inputProperties)
			for _, target := range targets {
				if delegationReferences(inputProperties["parameters"], target) {
					return nil, false, fmt.Errorf("module %q desired inputs depend on computed target output %q", moduleSymbol, target)
				}
			}
			for prerequisite := range inputTemplate["resources"].(map[string]any) {
				for _, target := range targets {
					targetDeclaration := declarations[target].(map[string]any)
					targetType, _, _ := strings.Cut(fmt.Sprint(targetDeclaration["type"]), "@")
					if delegationModuleIdentity("[reference('"+prerequisite+"').id]", child, arguments, childResources, declarations, scope, targetType) == target {
						return nil, false, fmt.Errorf("module %q desired inputs depend on computed target output %q", moduleSymbol, prerequisite)
					}
				}
				definition := definitions[path+moduleSymbol+"/"+prerequisite]
				if definition != nil && (definition.Driver == "delegated" || len(definition.AcceptsDelegationFrom) > 0) {
					return nil, false, fmt.Errorf("module %q delegation input prerequisite %q requires its own delegation group", moduleSymbol, prerequisite)
				}
			}
			inputDependencies := []string{}
			for _, dependency := range stringValues(module["dependsOn"]) {
				if !slices.Contains(targets, dependency) {
					inputDependencies = append(inputDependencies, dependency)
				}
			}
			inputModule["dependsOn"] = inputDependencies
			moduleDependencies := append(stringValues(module["dependsOn"]), targets...)
			moduleDependencies = append(moduleDependencies, inputSymbol)
			module["dependsOn"] = moduleDependencies
			declarations[inputSymbol] = inputModule
			changed = true
		}
		childChanged, err := planDelegationScope(child, scope, path+moduleSymbol+"/", childDefinitions, schemas)
		if err != nil {
			return nil, false, err
		}
		changed = changed || childChanged
	}
	return inputs, changed, nil
}

// delegationModuleIdentity follows only compiled identity aliases, never computed resource values.
func delegationModuleIdentity(value any, child, arguments, childResources, parents map[string]any, scope, targetType string) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	variables, _ := child["variables"].(map[string]any)
	seen := map[string]bool{}
	for strings.HasPrefix(text, "[variables('") && strings.HasSuffix(text, "')]") {
		key := text[len("[variables('") : len(text)-len("')]")]
		if seen[key] {
			return ""
		}
		seen[key] = true
		text, _ = variables[key].(string)
	}
	if strings.HasPrefix(text, "[parameters('") && strings.HasSuffix(text, "')]") {
		key := text[len("[parameters('") : len(text)-len("')]")]
		argument, _ := arguments[key].(map[string]any)
		text, _ = argument["value"].(string)
		if match := symbolicIDReference.FindStringSubmatch(text); match != nil {
			parent, _ := parents[match[1]].(map[string]any)
			typeName, _, _ := strings.Cut(fmt.Sprint(parent["type"]), "@")
			if strings.EqualFold(typeName, targetType) {
				return match[1]
			}
		}
	}
	if match := symbolicIDReference.FindStringSubmatch(text); match != nil {
		local, _ := childResources[match[1]].(map[string]any)
		typeName, _, _ := strings.Cut(fmt.Sprint(local["type"]), "@")
		if local["existing"] != true || !strings.EqualFold(typeName, targetType) {
			return ""
		}
		body, _ := local["properties"].(map[string]any)
		text, _ = body["name"].(string)
		if strings.HasPrefix(text, "[parameters('") && strings.HasSuffix(text, "')]") {
			key := text[len("[parameters('") : len(text)-len("')]")]
			argument, _ := arguments[key].(map[string]any)
			text, _ = argument["value"].(string)
		}
		for symbol, raw := range parents {
			parent, _ := raw.(map[string]any)
			parentType, _, _ := strings.Cut(fmt.Sprint(parent["type"]), "@")
			parentBody, _ := parent["properties"].(map[string]any)
			if text != "" && parentBody["name"] == text && strings.EqualFold(parentType, targetType) {
				return symbol
			}
		}
	}
	for symbol, raw := range parents {
		parent, _ := raw.(map[string]any)
		typeName, _, _ := strings.Cut(fmt.Sprint(parent["type"]), "@")
		body, _ := parent["properties"].(map[string]any)
		name, _ := body["name"].(string)
		if strings.EqualFold(typeName, targetType) && name != "" && !strings.HasPrefix(name, "[") &&
			strings.EqualFold(text, scope+"/providers/"+typeName+"/"+name) {
			return symbol
		}
	}
	return ""
}

func delegationInputPrerequisites(input, declarations map[string]any, groups map[string][]string, prerequisites []string) error {
	// Prerequisite resources execute once in the input module. The publication module keeps their logical GETs.
	delegates := []string{}
	for _, sources := range groups {
		delegates = append(delegates, sources...)
	}
	inputResources := input["resources"].(map[string]any)
	visiting := map[string]bool{}
	var scan func(any) error
	var visit func(string) error
	visit = func(symbol string) error {
		if slices.Contains(delegates, symbol) {
			return fmt.Errorf("desired delegation inputs depend on delegate output %q", symbol)
		}
		if visiting[symbol] {
			return fmt.Errorf("cyclic delegation input prerequisite %q", symbol)
		}
		if _, exists := inputResources[symbol]; exists {
			return nil
		}
		declaration, _ := declarations[symbol].(map[string]any)
		if declaration == nil {
			return fmt.Errorf("unknown delegation input prerequisite %q", symbol)
		}
		if delegationModuleTemplate(declaration) != nil {
			return fmt.Errorf("nested module prerequisite %q cannot be split as a native resource", symbol)
		}
		visiting[symbol] = true
		for _, dependency := range stringValues(declaration["dependsOn"]) {
			if err := visit(dependency); err != nil {
				return err
			}
			if err := scan(declaration["properties"]); err != nil {
				return err
			}
		}
		visiting[symbol] = false
		copy, err := cloneDelegationObject(declaration)
		if err != nil {
			return err
		}
		inputResources[symbol] = copy
		if declaration["existing"] != true {
			body, _ := declaration["properties"].(map[string]any)
			if declaration["import"] == nil || body["name"] == nil {
				return fmt.Errorf("prerequisite %q is not a native imported resource", symbol)
			}
			declaration["existing"] = true
			declaration["properties"] = map[string]any{"name": body["name"]}
			declaration["dependsOn"] = []string{}
		}
		return nil
	}
	scan = func(value any) error {
		switch value := value.(type) {
		case string:
			if !strings.HasPrefix(value, "[") || strings.HasPrefix(value, "[[") || !strings.HasSuffix(value, "]") {
				return nil
			}
			var err error
			rewriteARM(value[1:len(value)-1], func(call string) string {
				if strings.HasPrefix(call, "reference('") && err == nil {
					end := armQuotedEnd(call, len("reference("))
					symbol := call[len("reference('") : end-1]
					if _, exists := declarations[symbol]; exists {
						err = visit(symbol)
					}
				}
				return call
			})
			return err
		case map[string]any:
			for _, member := range value {
				if err := scan(member); err != nil {
					return err
				}
			}
		case []any:
			for _, member := range value {
				if err := scan(member); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, prerequisite := range prerequisites {
		if err := visit(prerequisite); err != nil {
			return err
		}
	}
	return scan(input["outputs"])
}

func delegationReferences(value any, symbol string) bool {
	switch value := value.(type) {
	case string:
		found := false
		if strings.HasPrefix(value, "[") && !strings.HasPrefix(value, "[[") && strings.HasSuffix(value, "]") {
			rewriteARM(value[1:len(value)-1], func(call string) string {
				selected, _, _ := delegationSymbolReference(call)
				found = found || selected == symbol || strings.HasPrefix(call, "reference('"+symbol+"')")
				return call
			})
		}
		return found
	case map[string]any:
		for _, member := range value {
			if delegationReferences(member, symbol) {
				return true
			}
		}
	case []any:
		for _, member := range value {
			if delegationReferences(member, symbol) {
				return true
			}
		}
	}
	return false
}

// pruneDelegationInputParameters excludes output-only parameter dependencies from the input deployment.
// The publication module keeps the original parameters and variables unchanged.
func pruneDelegationInputParameters(template, moduleProperties map[string]any) {
	parameters, _ := template["parameters"].(map[string]any)
	variables, _ := template["variables"].(map[string]any)
	usedParameters := map[string]bool{}
	usedVariables := map[string]bool{}
	var scan func(any)
	scan = func(value any) {
		switch value := value.(type) {
		case string:
			if !strings.HasPrefix(value, "[") || strings.HasPrefix(value, "[[") || !strings.HasSuffix(value, "]") {
				return
			}
			rewriteARM(value[1:len(value)-1], func(call string) string {
				for _, function := range []string{"parameters", "variables"} {
					arguments := delegationARMArguments(call, function)
					if len(arguments) != 1 || !strings.HasPrefix(arguments[0], "'") || armQuotedEnd(arguments[0], 0) != len(arguments[0]) {
						continue
					}
					key := strings.ReplaceAll(arguments[0][1:len(arguments[0])-1], "''", "'")
					if function == "parameters" && !usedParameters[key] {
						usedParameters[key] = true
						scan(parameters[key])
					} else if function == "variables" && !usedVariables[key] {
						usedVariables[key] = true
						scan(variables[key])
						copies, _ := variables["copy"].([]any)
						for _, raw := range copies {
							copy, _ := raw.(map[string]any)
							if copy["name"] == key {
								scan(copy)
							}
						}
					}
				}
				return call
			})
		case map[string]any:
			for _, member := range value {
				scan(member)
			}
		case []any:
			for _, member := range value {
				scan(member)
			}
		}
	}
	scan(template["resources"])
	scan(template["outputs"])
	scan(template["imports"])
	arguments, _ := moduleProperties["parameters"].(map[string]any)
	for key := range parameters {
		if !usedParameters[key] {
			delete(parameters, key)
			delete(arguments, key)
		}
	}
	for key := range variables {
		copies, isCopy := variables[key].([]any)
		if key == "copy" && isCopy {
			selected := []any{}
			for _, raw := range copies {
				copy, _ := raw.(map[string]any)
				name, _ := copy["name"].(string)
				if usedVariables[name] {
					selected = append(selected, copy)
				}
			}
			if len(selected) == 0 {
				delete(variables, key)
			} else {
				variables[key] = selected
			}
		} else if !usedVariables[key] {
			delete(variables, key)
		}
	}
}
