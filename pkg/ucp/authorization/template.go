// Copyright 2026 The Radius Authors.
// Licensed under the Apache License, Version 2.0.

package authorization

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ResolveDeploymentTemplate resolves the bounded, side-effect-free expression
// subset accepted by user-RBAC preflight. The engine receives these same resolved
// inputs, rather than independently evaluating expressions after authorization.
func ResolveDeploymentTemplate(template, parameters map[string]any, group string) (map[string]any, error) {
	copy, err := clonePolicyValue(template)
	if err != nil {
		return nil, err
	}
	for key := range copy {
		if !slices.Contains([]string{"$schema", "contentVersion", "languageVersion", "metadata", "imports", "extensions", "resources", "parameters", "variables", "outputs", "functions", "definitions"}, key) {
			return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported template field %s", key)
		}
	}
	r := &templateResolver{
		template: copy, parameters: parameters, group: group,
		resources: map[string]map[string]any{}, resolved: map[string]map[string]any{},
		active: map[string]bool{}, values: map[string]any{}, budget: 100000,
	}
	switch resources := copy["resources"].(type) {
	case []any:
		for index, raw := range resources {
			value, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid: template resource must be an object")
			}
			r.resources[strconv.Itoa(index)] = value
		}
	case map[string]any:
		for key, raw := range resources {
			value, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid: template resource must be an object")
			}
			r.resources[key] = value
		}
	default:
		return nil, fmt.Errorf("AuthorizationIndeterminate: inline template resources are required")
	}
	if len(r.resources) > 100 {
		return nil, fmt.Errorf("AuthorizationIndeterminate: template exceeds 100 resources")
	}
	declarations, _ := copy["parameters"].(map[string]any)
	normalizedParameters := map[string]any{}
	for name, value := range parameters {
		match := ""
		for declared := range declarations {
			if strings.EqualFold(name, declared) {
				if match != "" {
					return nil, fmt.Errorf("Invalid: ambiguous template parameter %s", name)
				}
				match = declared
			}
		}
		if match == "" {
			return nil, fmt.Errorf("Invalid: undeclared template parameter %s", name)
		}
		if _, exists := normalizedParameters[match]; exists {
			return nil, fmt.Errorf("Invalid: duplicate template parameter %s", name)
		}
		normalizedParameters[match] = value
	}
	r.parameters = normalizedParameters
	for name := range declarations {
		if _, err := r.named("parameters", name); err != nil {
			return nil, err
		}
	}
	for _, field := range []string{"functions", "definitions"} {
		if value, ok := copy[field]; ok {
			switch value := value.(type) {
			case []any:
				if len(value) == 0 {
					continue
				}
			case map[string]any:
				if len(value) == 0 {
					continue
				}
			}
			return nil, fmt.Errorf("AuthorizationIndeterminate: template %s is unsupported by preflight", field)
		}
	}
	items := []any{}
	for _, name := range slices.Sorted(maps.Keys(r.resources)) {
		resource, err := r.resource(name)
		if err != nil {
			return nil, err
		}
		if existing, _ := r.resources[name]["existing"].(bool); existing {
			continue
		}
		items = append(items, resource)
	}
	if err := r.validateDependencies(); err != nil {
		return nil, err
	}
	outputs := map[string]any{}
	if raw, exists := copy["outputs"]; exists {
		object, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid: template outputs must be an object")
		}
		for name, value := range object {
			resolved, err := r.value(value, 0)
			if err != nil {
				return nil, fmt.Errorf("output %s: %w", name, err)
			}
			outputs[name] = resolved
		}
	}
	result := map[string]any{"resources": items, "outputs": outputs}
	bytes, nodes := 4*1024*1024, 100000
	if err := resolvedTemplateSize(result, &bytes, &nodes, 0); err != nil {
		return nil, err
	}
	return result, nil
}

func resolvedTemplateSize(value any, bytes, nodes *int, depth int) error {
	*nodes--
	if *nodes < 0 || *bytes < 0 || depth > 64 {
		return fmt.Errorf("AuthorizationIndeterminate: resolved template exceeds preflight limits")
	}
	switch value := value.(type) {
	case map[string]any:
		*bytes -= 2 + 2*len(value)
		for key, child := range value {
			*bytes -= len(key) + 2
			if err := resolvedTemplateSize(child, bytes, nodes, depth+1); err != nil {
				return err
			}
		}
	case []any:
		*bytes -= 2 + len(value)
		for _, child := range value {
			if err := resolvedTemplateSize(child, bytes, nodes, depth+1); err != nil {
				return err
			}
		}
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		*bytes -= len(raw)
	}
	if *bytes < 0 {
		return fmt.Errorf("AuthorizationIndeterminate: resolved template exceeds preflight size limit")
	}
	return nil
}

type templateResolver struct {
	template, parameters map[string]any
	group                string
	resources, resolved  map[string]map[string]any
	active               map[string]bool
	values               map[string]any
	budget               int
	nesting              int
}

func (r *templateResolver) value(value any, depth int) (any, error) {
	r.budget--
	r.nesting++
	defer func() { r.nesting-- }()
	if depth > 64 || r.nesting > 64 || r.budget < 0 {
		return nil, fmt.Errorf("AuthorizationIndeterminate: template expression complexity exceeds preflight limits")
	}
	switch value := value.(type) {
	case string:
		if strings.HasPrefix(value, "[[") {
			return value[1:], nil
		}
		if !strings.HasPrefix(value, "[") {
			return value, nil
		}
		if !strings.HasSuffix(value, "]") {
			return nil, fmt.Errorf("Invalid: malformed template expression")
		}
		p := expressionParser{resolver: r, text: value[1 : len(value)-1], depth: depth + 1}
		result, err := p.expression()
		if err != nil {
			return nil, err
		}
		p.space()
		if p.position != len(p.text) {
			return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported template expression syntax")
		}
		return result, nil
	case map[string]any:
		result := map[string]any{}
		for _, key := range slices.Sorted(maps.Keys(value)) {
			child, err := r.value(value[key], depth+1)
			if err != nil {
				return nil, err
			}
			result[key] = child
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		for index, child := range value {
			resolved, err := r.value(child, depth+1)
			if err != nil {
				return nil, err
			}
			result[index] = resolved
		}
		return result, nil
	default:
		return value, nil
	}
}

func (r *templateResolver) named(kind, name string) (any, error) {
	key := kind + ":" + name
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	if r.active[key] {
		return nil, fmt.Errorf("Invalid: cyclic template %s %s", kind, name)
	}
	r.active[key] = true
	defer delete(r.active, key)
	declarations, _ := r.template[kind].(map[string]any)
	declaration, ok := declarations[name]
	if !ok {
		return nil, fmt.Errorf("Invalid: unknown template %s %s", kind, name)
	}
	var value any
	if kind == "parameters" {
		definition, ok := declaration.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid: malformed parameter %s", name)
		}
		for field := range definition {
			if !slices.Contains([]string{"type", "defaultValue", "allowedValues", "metadata", "minLength", "maxLength", "minValue", "maxValue"}, field) {
				return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported parameter field %s", field)
			}
		}
		if supplied, exists := r.parameters[name]; exists {
			input, ok := supplied.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("Invalid: parameter %s requires a value object", name)
			}
			for field := range input {
				if field != "value" {
					return nil, fmt.Errorf("Invalid: parameter references are not supported")
				}
			}
			value, ok = input["value"]
			if !ok {
				return nil, fmt.Errorf("Invalid: parameter %s requires a value", name)
			}
		} else {
			var exists bool
			value, exists = definition["defaultValue"]
			if !exists {
				return nil, fmt.Errorf("Invalid: missing template parameter %s", name)
			}
			resolved, err := r.value(value, 0)
			if err != nil {
				return nil, err
			}
			value = resolved
		}
		typ, _ := definition["type"].(string)
		valid := false
		switch strings.ToLower(typ) {
		case "string", "securestring":
			_, valid = value.(string)
		case "object", "secureobject":
			_, valid = value.(map[string]any)
		case "array":
			_, valid = value.([]any)
		case "bool":
			_, valid = value.(bool)
		case "int":
			number, ok := value.(float64)
			valid = ok && number == float64(int64(number))
		}
		if !valid {
			return nil, fmt.Errorf("Invalid: parameter %s does not match type %s", name, typ)
		}
		if raw, exists := definition["allowedValues"]; exists {
			allowed, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("Invalid: parameter %s allowedValues must be an array", name)
			}
			if !slices.ContainsFunc(allowed, func(candidate any) bool { return reflect.DeepEqual(candidate, value) }) {
				return nil, fmt.Errorf("Invalid: parameter %s is not an allowed value", name)
			}
		}
		if err := validateParameterBounds(name, definition, value); err != nil {
			return nil, err
		}
	} else {
		var err error
		value, err = r.value(declaration, 0)
		if err != nil {
			return nil, err
		}
	}
	r.values[key] = value
	return value, nil
}

func validateParameterBounds(name string, definition map[string]any, value any) error {
	for _, field := range []string{"minLength", "maxLength", "minValue", "maxValue"} {
		raw, exists := definition[field]
		if !exists {
			continue
		}
		bound, ok := raw.(float64)
		if !ok || bound != float64(int64(bound)) {
			return fmt.Errorf("Invalid: parameter %s %s must be an integer", name, field)
		}
		var actual float64
		if strings.HasSuffix(field, "Length") {
			if bound < 0 {
				return fmt.Errorf("Invalid: parameter %s %s must not be negative", name, field)
			}
			switch value := value.(type) {
			case string:
				actual = float64(utf8.RuneCountInString(value))
			case []any:
				actual = float64(len(value))
			default:
				return fmt.Errorf("Invalid: parameter %s %s requires a string or array", name, field)
			}
		} else {
			actual, ok = value.(float64)
			if !ok {
				return fmt.Errorf("Invalid: parameter %s %s requires an integer", name, field)
			}
		}
		if strings.HasPrefix(field, "min") && actual < bound || strings.HasPrefix(field, "max") && actual > bound {
			return fmt.Errorf("Invalid: parameter %s violates %s", name, field)
		}
	}
	return nil
}

func (r *templateResolver) resource(name string) (map[string]any, error) {
	if result, ok := r.resolved[name]; ok {
		return result, nil
	}
	key := "resource:" + name
	if r.active[key] {
		return nil, fmt.Errorf("Invalid: cyclic template resource %s", name)
	}
	r.active[key] = true
	defer delete(r.active, key)
	raw, ok := r.resources[name]
	if !ok {
		return nil, fmt.Errorf("AuthorizationIndeterminate: reference must identify a resource declared in this template")
	}
	for _, field := range []string{"copy", "condition", "resources", "subscriptionId", "resourceGroup"} {
		if _, exists := raw[field]; exists {
			return nil, fmt.Errorf("AuthorizationIndeterminate: resource %s is unsupported", field)
		}
	}
	var envelope map[string]any
	typeName, _ := raw["type"].(string)
	version, _ := raw["apiVersion"].(string)
	if strings.Contains(typeName, "@") {
		parts := strings.Split(typeName, "@")
		if len(parts) != 2 {
			return nil, fmt.Errorf("Invalid: malformed resource type")
		}
		typeName, version = parts[0], parts[1]
		alias, _ := raw["import"].(string)
		if alias == "" {
			alias, _ = raw["extension"].(string)
		}
		if !strings.EqualFold(alias, "radius") {
			return nil, fmt.Errorf("AuthorizationIndeterminate: only Radius extension resources are supported")
		}
		envelope, ok = raw["properties"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Invalid: Radius resource envelope required")
		}
	} else {
		envelope = maps.Clone(raw)
		delete(envelope, "type")
		delete(envelope, "apiVersion")
	}
	dependencies := raw["dependsOn"]
	delete(envelope, "dependsOn")
	delete(envelope, "existing")
	resolved, err := r.value(envelope, 0)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %w", name, err)
	}
	result := resolved.(map[string]any)
	result["type"], result["apiVersion"] = typeName, version
	if !strings.EqualFold(typeName, "Radius.Core/roleDefinitions") && !strings.EqualFold(typeName, "Radius.Core/roleAssignments") {
		if _, exists := result["location"]; !exists {
			result["location"] = "global"
		}
	}
	if scope, exists := result["scope"]; exists {
		if object, ok := scope.(map[string]any); ok {
			scope = object["id"]
		}
		scope, ok := scope.(string)
		if !ok || scope == "" {
			return nil, fmt.Errorf("Invalid: assignment scope must be a resource ID")
		}
		result["scope"] = scope
	}
	var id string
	if existing, _ := raw["existing"].(bool); existing && strings.EqualFold(typeName, "Radius.Core/roleDefinitions") && result["scope"] != nil {
		scope, _ := result["scope"].(string)
		contains, scopeError := ScopeContains(scope, r.group)
		if scopeError != nil || !contains {
			return nil, fmt.Errorf("ScopeNotAssignable: existing roles must be at an ancestor scope")
		}
		name, _ := result["name"].(string)
		if name == "" || strings.ContainsAny(name, "/[]?#") {
			return nil, fmt.Errorf("Invalid: role name required")
		}
		id, err = canonicalTarget(strings.TrimRight(scope, "/") + "/providers/" + typeName + "/" + name)
	} else {
		id, err = DeploymentResourceID(r.group, result)
	}
	if err != nil {
		return nil, err
	}
	// Publish identity before traversing dependencies. Classic ID dependencies
	// may inspect a resource which depends on this one without creating a cycle.
	r.resolved[name] = result
	r.values["id:"+name] = id
	if dependencies != nil {
		values, ok := dependencies.([]any)
		if !ok {
			return nil, fmt.Errorf("Invalid: resource dependencies must be an array")
		}
		dependsOn := []any{}
		for _, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("Invalid: dependency must identify a declared resource")
			}
			if _, symbolic := r.resources[name]; !symbolic {
				target := name
				if strings.HasPrefix(name, "[") {
					resolved, err := r.value(name, 0)
					if err != nil {
						return nil, err
					}
					target, ok = resolved.(string)
					if !ok {
						return nil, fmt.Errorf("Invalid: dependency must be a resource ID")
					}
				}
				match := ""
				for candidate := range r.resources {
					_, err := r.resource(candidate)
					if err != nil {
						return nil, err
					}
					if strings.EqualFold(target, fmt.Sprint(r.values["id:"+candidate])) {
						match = candidate
						break
					}
				}
				name = match
			}
			dependency, exists := r.resources[name]
			if !exists {
				return nil, fmt.Errorf("AuthorizationIndeterminate: dependency must identify a declared resource")
			}
			if _, err := r.resource(name); err != nil {
				return nil, err
			}
			if existing, _ := dependency["existing"].(bool); !existing {
				dependsOn = append(dependsOn, r.values["id:"+name])
			}
		}
		result["dependsOn"] = dependsOn
	}
	return result, nil
}

func (r *templateResolver) validateDependencies() error {
	dependencies := map[string][]any{}
	for name, resource := range r.resolved {
		id := r.values["id:"+name].(string)
		dependencies[id], _ = resource["dependsOn"].([]any)
	}
	states := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if states[id] == 1 {
			return fmt.Errorf("Invalid: cyclic template resource dependencies at %s", id)
		}
		if states[id] == 2 {
			return nil
		}
		states[id] = 1
		for _, dependency := range dependencies[id] {
			if err := visit(dependency.(string)); err != nil {
				return err
			}
		}
		states[id] = 2
		return nil
	}
	for id := range dependencies {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

// DeploymentResourceID computes the actual policy/workload target, including an
// assignment's optional resource scope.
func DeploymentResourceID(group string, resource map[string]any) (string, error) {
	typ, _ := resource["type"].(string)
	name, _ := resource["name"].(string)
	version, _ := resource["apiVersion"].(string)
	if name == "" || strings.ContainsAny(name, "/[]?#") || version == "" || strings.Count(typ, "/") != 1 || strings.HasPrefix(strings.ToLower(typ), "microsoft.") {
		return "", fmt.Errorf("AuthorizationIndeterminate: only named, top-level Radius resources are supported")
	}
	scope := group
	if raw, exists := resource["scope"]; exists {
		var ok bool
		scope, ok = raw.(string)
		if !ok || !strings.EqualFold(typ, "Radius.Core/roleAssignments") {
			return "", fmt.Errorf("AuthorizationIndeterminate: only assignments support resource scope")
		}
		canonical, err := canonicalTarget(scope)
		if err != nil {
			return "", err
		}
		contains, err := ScopeContains(group, canonical)
		if err != nil || !contains {
			return "", fmt.Errorf("ScopeNotAssignable: assignment scope must be in the deployment resource group")
		}
		scope = canonical
	}
	return canonicalTarget(strings.TrimRight(scope, "/") + "/providers/" + typ + "/" + name)
}

type expressionParser struct {
	resolver        *templateResolver
	text            string
	position, depth int
}

func (p *expressionParser) space() {
	for p.position < len(p.text) && unicode.IsSpace(rune(p.text[p.position])) {
		p.position++
	}
}

func (p *expressionParser) consume(value byte) bool {
	p.space()
	if p.position < len(p.text) && p.text[p.position] == value {
		p.position++
		return true
	}
	return false
}

func (p *expressionParser) identifier() string {
	p.space()
	start := p.position
	for p.position < len(p.text) {
		c := p.text[p.position]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			break
		}
		p.position++
	}
	return p.text[start:p.position]
}

func (p *expressionParser) expression() (any, error) {
	p.depth++
	defer func() { p.depth-- }()
	p.resolver.budget--
	if p.depth > 64 || p.resolver.budget < 0 {
		return nil, fmt.Errorf("AuthorizationIndeterminate: expression complexity exceeds preflight limits")
	}
	p.space()
	var value any
	if p.consume('\'') {
		var text strings.Builder
		closed := false
		for p.position < len(p.text) {
			char := p.text[p.position]
			p.position++
			if char == '\'' {
				if p.position < len(p.text) && p.text[p.position] == '\'' {
					p.position++
					text.WriteByte('\'')
					continue
				}
				closed = true
				break
			}
			text.WriteByte(char)
		}
		if !closed {
			return nil, fmt.Errorf("Invalid: unterminated expression string")
		}
		value = text.String()
	} else {
		name := p.identifier()
		if name == "" {
			return nil, fmt.Errorf("AuthorizationIndeterminate: unsupported expression operand")
		}
		if p.consume('(') {
			args := []any{}
			if !p.consume(')') {
				for {
					arg, err := p.expression()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					if p.consume(')') {
						break
					}
					if !p.consume(',') {
						return nil, fmt.Errorf("Invalid: malformed expression arguments")
					}
				}
			}
			var err error
			value, err = p.call(strings.ToLower(name), args)
			if err != nil {
				return nil, err
			}
		} else if name == "true" || name == "false" {
			value = name == "true"
		} else if name == "null" {
			value = nil
		} else {
			number, err := strconv.ParseFloat(name, 64)
			if err != nil {
				return nil, fmt.Errorf("AuthorizationIndeterminate: unknown expression operand %s", name)
			}
			value = number
		}
	}
	for {
		var key any
		if p.consume('.') {
			key = p.identifier()
		} else if p.consume('[') {
			var err error
			key, err = p.expression()
			if err != nil {
				return nil, err
			}
			if !p.consume(']') {
				return nil, fmt.Errorf("Invalid: malformed expression index")
			}
		} else {
			break
		}
		switch object := value.(type) {
		case map[string]any:
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("Invalid: object property requires a string")
			}
			var exists bool
			value, exists = object[name]
			if !exists {
				return nil, fmt.Errorf("AuthorizationIndeterminate: referenced property %s is unavailable during preflight", name)
			}
		case []any:
			index, ok := key.(float64)
			if !ok || index != float64(int(index)) || index < 0 || index >= float64(len(object)) {
				return nil, fmt.Errorf("Invalid: array index is out of bounds")
			}
			value = object[int(index)]
		default:
			return nil, fmt.Errorf("Invalid: expression property access requires an object")
		}
	}
	return value, nil
}

func (p *expressionParser) call(name string, args []any) (any, error) {
	stringsOnly := func() ([]string, error) {
		result := make([]string, len(args))
		size := 0
		for index, arg := range args {
			text, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("Invalid: %s requires string arguments", name)
			}
			size += len(text)
			if size > 4*1024*1024 {
				return nil, fmt.Errorf("AuthorizationIndeterminate: expression result exceeds preflight size limit")
			}
			result[index] = text
		}
		return result, nil
	}
	switch name {
	case "parameters", "variables":
		values, err := stringsOnly()
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("Invalid: %s requires one name", name)
		}
		return p.resolver.named(name, values[0])
	case "resourcegroup":
		if len(args) != 0 {
			return nil, fmt.Errorf("Invalid: resourceGroup accepts no arguments")
		}
		parts := strings.Split(p.resolver.group, "/")
		return map[string]any{"id": p.resolver.group, "name": parts[len(parts)-1]}, nil
	case "reference":
		values, err := stringsOnly()
		if err != nil {
			return nil, err
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("Invalid: unsupported reference arguments")
		}
		resource, err := p.resolver.resource(values[0])
		if err != nil {
			return nil, err
		}
		result := maps.Clone(resource)
		result["id"] = p.resolver.values["id:"+values[0]]
		return result, nil
	case "concat", "guid", "tolower", "toupper":
		values, err := stringsOnly()
		if err != nil {
			return nil, err
		}
		switch name {
		case "concat":
			return strings.Join(values, ""), nil
		case "guid":
			if len(values) == 0 {
				return nil, fmt.Errorf("Invalid: guid requires at least one argument")
			}
			namespace := uuid.MustParse("11fb06fb-712d-4ddd-98c7-e71bbd588830")
			return uuid.NewSHA1(namespace, []byte(strings.Join(values, "-"))).String(), nil
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("Invalid: %s requires one argument", name)
		}
		if name == "tolower" {
			return strings.ToLower(values[0]), nil
		}
		return strings.ToUpper(values[0]), nil
	case "format":
		if len(args) == 0 {
			return nil, fmt.Errorf("Invalid: format requires a format string")
		}
		format, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("Invalid: format requires a string")
		}
		var result strings.Builder
		for index := 0; index < len(format); {
			char := format[index]
			if (char == '{' || char == '}') && index+1 < len(format) && format[index+1] == char {
				result.WriteByte(char)
				index += 2
				continue
			}
			if char == '}' {
				return nil, fmt.Errorf("Invalid: unmatched format brace")
			}
			if char != '{' {
				result.WriteByte(char)
				index++
				continue
			}
			end := strings.IndexByte(format[index+1:], '}')
			if end < 0 {
				return nil, fmt.Errorf("Invalid: unmatched format brace")
			}
			number, err := strconv.Atoi(format[index+1 : index+1+end])
			if err != nil || number < 0 || number+1 >= len(args) {
				return nil, fmt.Errorf("AuthorizationIndeterminate: only positional format placeholders are supported")
			}
			text, ok := args[number+1].(string)
			if !ok {
				return nil, fmt.Errorf("Invalid: format argument must be a string")
			}
			if result.Len()+len(text) > 4*1024*1024 {
				return nil, fmt.Errorf("AuthorizationIndeterminate: format result exceeds preflight size limit")
			}
			result.WriteString(text)
			index += end + 2
		}
		return result.String(), nil
	case "resourceid", "extensionresourceid":
		values, err := stringsOnly()
		if err != nil {
			return nil, err
		}
		scope := p.resolver.group
		if name == "extensionresourceid" {
			if len(values) < 3 {
				return nil, fmt.Errorf("Invalid: extensionResourceId requires scope, type and name")
			}
			scope, values = values[0], values[1:]
		}
		if len(values) != 2 || strings.Count(values[0], "/") != 1 {
			return nil, fmt.Errorf("AuthorizationIndeterminate: only single-name resource IDs are supported")
		}
		return canonicalTarget(strings.TrimRight(scope, "/") + "/providers/" + values[0] + "/" + values[1])
	}
	return nil, fmt.Errorf("AuthorizationIndeterminate: expression function %s is not supported by preflight", name)
}
