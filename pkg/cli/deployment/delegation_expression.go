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
	"fmt"
	"strings"
)

// rewriteARM visits function calls, not text inside ARM string literals. It does not evaluate expressions.
func rewriteARM(expression string, rewrite func(string) string) string {
	var result strings.Builder
	for i := 0; i < len(expression); {
		start := i
		if expression[i] == '\'' {
			i = armQuotedEnd(expression, i)
		} else if armIdentifier(expression[i]) {
			for i < len(expression) && armIdentifier(expression[i]) {
				i++
			}
			if i < len(expression) && expression[i] == '(' {
				open := i
				depth := 1
				i++
				for i < len(expression) && depth > 0 {
					switch expression[i] {
					case '\'':
						i = armQuotedEnd(expression, i)
						continue
					case '(':
						depth++
					case ')':
						depth--
					}
					i++
				}
				if depth == 0 {
					call := expression[start:open+1] + rewriteARM(expression[open+1:i-1], rewrite) + ")"
					if strings.HasPrefix(expression[i:], ".id") && (i+3 == len(expression) || !armIdentifier(expression[i+3])) {
						call += ".id"
						i += 3
					}
					result.WriteString(rewrite(call))
					continue
				}
			}
		} else {
			i++
		}
		result.WriteString(expression[start:i])
	}
	return result.String()
}

func armIdentifier(ch byte) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
}

func armQuotedEnd(text string, start int) int {
	for i := start + 1; i < len(text); i++ {
		if text[i] == '\'' {
			if i+1 < len(text) && text[i+1] == '\'' {
				i++
				continue
			}
			return i + 1
		}
	}
	return len(text)
}

func delegationMembers(declaration, member map[string]any, symbol string) (string, error) {
	return delegationMembersWithTarget(declaration, member, symbol, "", "")
}

func delegationMembersWithTarget(declaration, member map[string]any, symbol, sourceTargetIndex, targetCopyName string) (string, error) {
	expression, err := armObjectValue(member)
	if err != nil {
		return "", err
	}
	condition := "bool('true')"
	if value, present := declaration["condition"]; present {
		condition, err = armObjectValue(value)
		if err != nil {
			return "", err
		}
	}
	copy, _ := declaration["copy"].(map[string]any)
	if copy == nil {
		if targetCopyName != "" {
			condition = fmt.Sprintf("and(%s,equals(%s,copyIndex('%s')))", condition, sourceTargetIndex, strings.ReplaceAll(targetCopyName, "'", "''"))
		}
		return fmt.Sprintf("if(%s,createArray(%s),createArray())", condition, expression), nil
	}
	count, err := armObjectValue(copy["count"])
	if err != nil || copy["count"] == nil {
		return "", fmt.Errorf("delegation loop %q requires a count", symbol)
	}
	name, _ := copy["name"].(string)
	index := "__radiusDelegationIndex"
	for strings.Contains(expression+condition, "'"+index+"'") {
		index += "_"
	}
	expression, err = delegationCopyIndex(expression, name, index, true)
	if err != nil {
		return "", err
	}
	condition, err = delegationCopyIndex(condition, name, index, true)
	if err != nil {
		return "", err
	}
	var referenceErr error
	rewriteARM(expression+","+condition, func(call string) string {
		if strings.HasPrefix(call, "reference(") {
			rewriteARM(call, func(nested string) string {
				if nested == "lambdaVariables('"+index+"')" {
					referenceErr = fmt.Errorf("delegation loop %q has a computed indexed resource reference that cannot be evaluated inside a native lambda", symbol)
				}
				return nested
			})
		}
		return call
	})
	if referenceErr != nil {
		return "", referenceErr
	}
	if targetCopyName != "" {
		sourceTargetIndex, err = delegationCopyIndex(sourceTargetIndex, name, index, true)
		if err != nil {
			return "", err
		}
		condition = fmt.Sprintf("and(%s,equals(%s,copyIndex('%s')))", condition, sourceTargetIndex, strings.ReplaceAll(targetCopyName, "'", "''"))
	}
	return fmt.Sprintf("map(filter(range(0,%s),lambda('%s',%s)),lambda('%s',%s))", count, index, condition, index, expression), nil
}

func delegationCopyIndex(expression, name, index string, unnamed bool) (string, error) {
	return delegationCopyIndexValue(expression, name, "lambdaVariables('"+index+"')", unnamed)
}

func delegationCopyIndexValue(expression, name, replacement string, unnamed bool) (string, error) {
	var err error
	result := rewriteARM(expression, func(call string) string {
		if !strings.HasPrefix(call, "copyIndex(") || !strings.HasSuffix(call, ")") {
			return call
		}
		argument := strings.TrimSpace(call[len("copyIndex(") : len(call)-1])
		if strings.HasPrefix(argument, "'") {
			end := armQuotedEnd(argument, 0)
			if argument[:end] != "'"+strings.ReplaceAll(name, "'", "''")+"'" {
				if unnamed {
					err = fmt.Errorf("delegation input contains an unresolved copy index: %s", call)
				}
				return call
			}
			argument = strings.TrimSpace(argument[end:])
			argument = strings.TrimSpace(strings.TrimPrefix(argument, ","))
		} else if !unnamed {
			return call
		}
		value := replacement
		if argument != "" {
			value = "add(" + value + "," + argument + ")"
		}
		return value
	})
	return result, err
}

// delegationSymbolReference recognizes the indexed symbolic identities emitted by Bicep.
func delegationSymbolReference(call string) (symbol, index string, identity bool) {
	identity = strings.HasSuffix(call, ".id")
	arguments := delegationARMArguments(strings.TrimSuffix(call, ".id"), "reference")
	if len(arguments) != 1 {
		return "", "", false
	}
	value := arguments[0]
	if format := delegationARMArguments(value, "format"); len(format) == 2 {
		if strings.HasPrefix(format[0], "'") && strings.HasSuffix(format[0], "[{0}]'") &&
			armQuotedEnd(format[0], 0) == len(format[0]) {
			return strings.ReplaceAll(format[0][1:len(format[0])-len("[{0}]'")], "''", "'"), format[1], identity
		}
		return "", "", false
	}
	if !strings.HasPrefix(value, "'") || armQuotedEnd(value, 0) != len(value) {
		return "", "", false
	}
	symbol = strings.ReplaceAll(value[1:len(value)-1], "''", "'")
	if open := strings.LastIndex(symbol, "["); open >= 0 && strings.HasSuffix(symbol, "]") {
		index = symbol[open+1 : len(symbol)-1]
		if index == "" || strings.IndexFunc(index, func(ch rune) bool { return ch < '0' || ch > '9' }) >= 0 {
			return "", "", false
		}
		return symbol[:open], index, identity
	}
	return symbol, "", identity
}

func replaceIndexedTargetIdentity(value any, symbol, targetID, copyName string) (any, error) {
	var err error
	var replace func(any) any
	replace = func(value any) any {
		switch value := value.(type) {
		case string:
			if !strings.HasPrefix(value, "[") || strings.HasPrefix(value, "[[") || !strings.HasSuffix(value, "]") {
				return value
			}
			return "[" + rewriteARM(value[1:len(value)-1], func(call string) string {
				selected, index, identity := delegationSymbolReference(call)
				if selected != symbol || !identity || err != nil {
					return call
				}
				if index == "" {
					err = fmt.Errorf("copied delegation target %q requires an instance index", symbol)
					return call
				}
				expression := targetID
				if strings.HasPrefix(expression, "[") {
					expression, err = delegationCopyIndexValue(expression[1:len(expression)-1], copyName, index, true)
				} else {
					expression, err = armObjectValue(expression)
				}
				return expression
			}) + "]"
		case map[string]any:
			for key, member := range value {
				value[key] = replace(member)
			}
		case []any:
			for i, member := range value {
				value[i] = replace(member)
			}
		}
		return value
	}
	return replace(value), err
}

func delegationDependencySymbol(dependency string, declarations map[string]any) string {
	expression, err := armObjectValue(dependency)
	if err != nil {
		return dependency
	}
	symbol, index, _ := delegationSymbolReference("reference(" + expression + ")")
	resource, _ := declarations[symbol].(map[string]any)
	if index != "" && resource["copy"] != nil {
		return symbol
	}
	return dependency
}

func indexedDelegationSourceName(source, target map[string]any, index string) (string, error) {
	sourceBody, _ := source["properties"].(map[string]any)
	name, err := armObjectValue(sourceBody["name"])
	if err != nil {
		return "", err
	}
	targetCopy, _ := target["copy"].(map[string]any)
	copyName, _ := targetCopy["name"].(string)
	count, err := armObjectValue(targetCopy["count"])
	if err != nil {
		return "", err
	}
	condition := func(resource map[string]any) (string, error) {
		if value, present := resource["condition"]; present {
			return armObjectValue(value)
		}
		return "bool('true')", nil
	}
	targetCondition, err := condition(target)
	if err != nil {
		return "", err
	}
	targetCondition, err = delegationCopyIndexValue(targetCondition, copyName, index, true)
	if err != nil {
		return "", err
	}
	sourceCondition, err := condition(source)
	if err != nil {
		return "", err
	}
	// The name is unchanged, but evaluating it validates active references even when the target loop is empty.
	return fmt.Sprintf("[if(%s,last(createArray(range(0,if(%s,%s,0))[%s],%s)),%s)]", sourceCondition, targetCondition, count, index, name, name), nil
}
func delegationARMArguments(expression, function string) []string {
	prefix := function + "("
	if !strings.HasPrefix(expression, prefix) || !strings.HasSuffix(expression, ")") {
		return nil
	}
	body := expression[len(prefix) : len(expression)-1]
	result := []string{}
	start, depth := 0, 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\'':
			i = armQuotedEnd(body, i) - 1
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(body) {
		result = append(result, strings.TrimSpace(body[start:]))
	}
	return result
}

// delegationAnnotatedExpression reads constructor/lambda shape only; the engine evaluates active members.
func delegationAnnotatedExpression(expression string, field map[string]any, targetType string) []any {
	result := []any{}
	visit := func(expression string, field map[string]any) {
		result = append(result, annotatedValues("["+expression+"]", field, targetType)...)
	}
	if arguments := delegationARMArguments(expression, "if"); len(arguments) == 3 {
		visit(arguments[1], field)
		visit(arguments[2], field)
	} else if arguments := delegationARMArguments(expression, "createArray"); arguments != nil {
		items, _ := field["items"].(map[string]any)
		for _, argument := range arguments {
			visit(argument, items)
		}
	} else if arguments := delegationARMArguments(expression, "createObject"); len(arguments)%2 == 0 && len(arguments) > 0 {
		properties, _ := field["properties"].(map[string]any)
		for i := 0; i < len(arguments); i += 2 {
			key := arguments[i]
			if !strings.HasPrefix(key, "'") || armQuotedEnd(key, 0) != len(key) {
				continue
			}
			key = strings.ReplaceAll(key[1:len(key)-1], "''", "'")
			raw, exists := properties[key]
			if !exists {
				raw = field["additionalProperties"]
			}
			property, _ := raw.(map[string]any)
			visit(arguments[i+1], property)
		}
	} else if arguments := delegationARMArguments(expression, "map"); len(arguments) == 2 {
		lambda := delegationARMArguments(arguments[1], "lambda")
		items, _ := field["items"].(map[string]any)
		if len(lambda) >= 2 {
			visit(lambda[len(lambda)-1], items)
		}
	} else if arguments := delegationARMArguments(expression, "toObject"); len(arguments) == 3 {
		lambda := delegationARMArguments(arguments[2], "lambda")
		property, _ := field["additionalProperties"].(map[string]any)
		if len(lambda) >= 2 {
			visit(lambda[len(lambda)-1], property)
		}
	}
	return result
}
