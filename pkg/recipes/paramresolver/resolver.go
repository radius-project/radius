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

package paramresolver

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/radius-project/radius/pkg/recipes/recipecontext"
)

// expressionPattern matches {{context.*}} template expressions, including ternary expressions.
var expressionPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

// singleExpressionPattern matches a string that consists of exactly one {{...}} expression with no
// surrounding text (e.g. "{{context.resource.properties.port}}"). It is used to detect parameters
// whose entire value is a single expression so their original scalar type can be preserved.
var singleExpressionPattern = regexp.MustCompile(`^\s*\{\{([^}]+)\}\}\s*$`)

// ternaryPattern matches single-level ternary expressions: expr == "val" ? "trueResult" : "falseResult"
var ternaryPattern = regexp.MustCompile(`^\s*(.+?)\s*==\s*"([^"]*)"\s*\?\s*"([^"]*)"\s*:\s*"([^"]*)"\s*$`)

// ResolveParameterExpressions resolves {{context.*}} template expressions in recipe parameters.
// It traverses the parameter map recursively and replaces expressions with values from the
// recipe context. Unrecognized expressions are left unchanged so that misconfigurations surface
// as IaC engine errors rather than being silently masked.
func ResolveParameterExpressions(params map[string]any, ctx *recipecontext.Context) map[string]any {
	if params == nil {
		return nil
	}

	lookup := buildContextLookup(ctx)
	typedLookup := buildTypedContextLookup(ctx)
	result := make(map[string]any, len(params))
	for k, v := range params {
		result[k] = resolveValue(v, lookup, typedLookup)
	}
	return result
}

// resolveValue resolves template expressions in a single value. It handles strings, maps, and slices recursively.
func resolveValue(v any, lookup map[string]string, typedLookup map[string]any) any {
	switch val := v.(type) {
	case string:
		// When the entire value is a single {{...}} expression that maps to a typed context value,
		// preserve the original scalar type so typed module parameters (int, bool, object) are passed
		// through correctly instead of being coerced to a string. Interpolated strings and ternary
		// expressions fall back to string resolution below.
		if typed, ok := resolveTypedExpression(val, typedLookup); ok {
			return typed
		}
		return resolveString(val, lookup)
	case map[string]any:
		resolved := make(map[string]any, len(val))
		for k, inner := range val {
			resolved[k] = resolveValue(inner, lookup, typedLookup)
		}
		return resolved
	case []any:
		resolved := make([]any, len(val))
		for i, inner := range val {
			resolved[i] = resolveValue(inner, lookup, typedLookup)
		}
		return resolved
	default:
		return v
	}
}

// resolveTypedExpression returns the typed value for a string that consists of exactly one {{...}}
// expression (e.g. "{{context.resource.properties.port}}") when that expression maps to a typed
// context value. It returns (nil, false) for interpolated strings, ternary expressions, or
// expressions without a typed value, so those fall back to string resolution.
func resolveTypedExpression(s string, typedLookup map[string]any) (any, bool) {
	m := singleExpressionPattern.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}

	key := strings.TrimSpace(m[1])
	val, ok := typedLookup[key]
	return val, ok
}

// resolveString replaces all {{...}} expressions in a string with their resolved values.
func resolveString(s string, lookup map[string]string) string {
	return expressionPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Strip the surrounding {{ and }}.
		inner := match[2 : len(match)-2]

		// Try ternary evaluation first.
		if result, ok := evaluateTernary(inner, lookup); ok {
			return result
		}

		// Simple context path lookup.
		key := strings.TrimSpace(inner)
		if val, ok := lookup[key]; ok {
			return val
		}

		// Unrecognized expression — leave unchanged.
		return match
	})
}

// evaluateTernary evaluates a single-level ternary expression of the form:
// expr == "val" ? "trueResult" : "falseResult"
// It returns the resolved result and true if the expression is a valid ternary, or ("", false)
// otherwise. If the condition path cannot be resolved, the entire ternary is left unchanged.
func evaluateTernary(inner string, lookup map[string]string) (string, bool) {
	matches := ternaryPattern.FindStringSubmatch(inner)
	if matches == nil {
		return "", false
	}

	conditionPath := strings.TrimSpace(matches[1])
	expectedValue := matches[2]
	trueResult := matches[3]
	falseResult := matches[4]

	conditionValue, ok := lookup[conditionPath]
	if !ok {
		// Unresolvable condition — leave the entire expression unchanged.
		return fmt.Sprintf("{{%s}}", inner), true
	}

	if conditionValue == expectedValue {
		return trueResult, true
	}
	return falseResult, true
}

// buildContextLookup builds a flat key-value map from the recipe context for expression resolution.
// Keys use dot-separated paths (e.g., "context.resource.name", "context.runtime.kubernetes.namespace").
func buildContextLookup(ctx *recipecontext.Context) map[string]string {
	if ctx == nil {
		return map[string]string{}
	}

	lookup := map[string]string{
		"context.resource.name": ctx.Resource.Name,
		"context.resource.id":   ctx.Resource.ID,
		"context.resource.type": ctx.Resource.Type,

		"context.application.name": ctx.Application.Name,
		"context.application.id":   ctx.Application.ID,

		"context.environment.name": ctx.Environment.Name,
		"context.environment.id":   ctx.Environment.ID,
	}

	// Add runtime.kubernetes fields.
	if ctx.Runtime.Kubernetes != nil {
		lookup["context.runtime.kubernetes.namespace"] = ctx.Runtime.Kubernetes.Namespace
		lookup["context.runtime.kubernetes.environmentNamespace"] = ctx.Runtime.Kubernetes.EnvironmentNamespace
	}

	// Add Azure provider fields.
	if ctx.Azure != nil {
		lookup["context.azure.resourceGroup.name"] = ctx.Azure.ResourceGroup.Name
		lookup["context.azure.resourceGroup.id"] = ctx.Azure.ResourceGroup.ID
		lookup["context.azure.subscription.subscriptionId"] = ctx.Azure.Subscription.SubscriptionID
		lookup["context.azure.subscription.id"] = ctx.Azure.Subscription.ID
	}

	// Add AWS provider fields.
	if ctx.AWS != nil {
		lookup["context.aws.region"] = ctx.AWS.Region
		lookup["context.aws.account"] = ctx.AWS.Account
	}

	// Add dynamic resource properties (context.resource.properties.*).
	for key, val := range ctx.Resource.Properties {
		lookup[fmt.Sprintf("context.resource.properties.%s", key)] = fmt.Sprintf("%v", val)
	}

	// Add connection properties (context.resource.connections.<name>.*).
	for connName, conn := range ctx.Resource.Connections {
		prefix := fmt.Sprintf("context.resource.connections.%s", connName)
		lookup[prefix+".id"] = conn.ID
		lookup[prefix+".name"] = conn.Name
		lookup[prefix+".type"] = conn.Type
		for propKey, propVal := range conn.Properties {
			lookup[fmt.Sprintf("%s.properties.%s", prefix, propKey)] = fmt.Sprintf("%v", propVal)
		}
	}

	return lookup
}

// buildTypedContextLookup builds a lookup of dynamic resource and connection properties that preserves
// each value's original Go type. It is used to resolve single-expression parameters (e.g.
// "{{context.resource.properties.port}}") without coercing typed values to strings. Only the
// arbitrarily-typed sources (resource properties and connection properties) are included; all other
// context fields are strings and are resolved through the string lookup.
func buildTypedContextLookup(ctx *recipecontext.Context) map[string]any {
	typed := map[string]any{}
	if ctx == nil {
		return typed
	}

	for key, val := range ctx.Resource.Properties {
		typed[fmt.Sprintf("context.resource.properties.%s", key)] = val
	}

	for connName, conn := range ctx.Resource.Connections {
		prefix := fmt.Sprintf("context.resource.connections.%s", connName)
		for propKey, propVal := range conn.Properties {
			typed[fmt.Sprintf("%s.properties.%s", prefix, propKey)] = propVal
		}
	}

	return typed
}
