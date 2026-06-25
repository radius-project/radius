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

// conditionPattern matches a single ternary condition of the form: <context path> == "value".
var conditionPattern = regexp.MustCompile(`^\s*(.+?)\s*==\s*"([^"]*)"\s*$`)

// ResolvedParameters holds resolved recipe parameters together with metadata identifying which
// parameters resolved from secret material, so drivers can route those values through secure channels
// (ARM @secure parameters / Terraform sensitive variables).
type ResolvedParameters struct {
	// Values holds the resolved parameter values.
	Values map[string]any
	// SecureKeys is the set of top-level parameter names whose value resolved from a secret expression.
	SecureKeys map[string]bool
}

// ResolveParameterExpressions resolves {{context.*}} template expressions in recipe parameters.
// It traverses the parameter map recursively and replaces expressions with values from the
// recipe context. Unrecognized expressions are left unchanged so that misconfigurations surface
// as IaC engine errors rather than being silently masked.
//
// Secret expressions (context.resource.connections.<name>.secrets.<key>) are resolved from a separate
// secret lookup and may only be used as the entire value of a parameter; interpolating a secret into a
// surrounding string returns an error so partial cleartext cannot leak into a non-secure value. Each
// parameter whose value resolves from a secret is reported in SecureKeys.
func ResolveParameterExpressions(params map[string]any, ctx *recipecontext.Context) (ResolvedParameters, error) {
	if params == nil {
		return ResolvedParameters{}, nil
	}

	lookup := buildContextLookup(ctx)
	typedLookup := buildTypedContextLookup(ctx)
	secretLookup := buildSecretLookup(ctx)

	result := make(map[string]any, len(params))
	secureKeys := map[string]bool{}
	for k, v := range params {
		resolved, secure, err := resolveValue(v, lookup, typedLookup, secretLookup)
		if err != nil {
			return ResolvedParameters{}, fmt.Errorf("failed to resolve parameter %q: %w", k, err)
		}
		result[k] = resolved
		if secure {
			secureKeys[k] = true
		}
	}
	return ResolvedParameters{Values: result, SecureKeys: secureKeys}, nil
}

// resolveValue resolves template expressions in a single value. It handles strings, maps, and slices
// recursively. The returned secure flag reports whether the value (or any nested value) resolved from a
// secret expression, so the caller can tag the top-level parameter for secure routing.
func resolveValue(v any, lookup map[string]string, typedLookup map[string]any, secretLookup map[string]string) (any, bool, error) {
	switch val := v.(type) {
	case string:
		// A secret may only be referenced as the entire value of a parameter. When the whole string is a
		// single {{...secrets...}} expression, inject the secret value and tag the parameter secure.
		if m := singleExpressionPattern.FindStringSubmatch(val); m != nil {
			if secret, ok := secretLookup[strings.TrimSpace(m[1])]; ok {
				return secret, true, nil
			}
		}
		// When the entire value is a single {{...}} expression that maps to a typed context value,
		// preserve the original scalar type so typed module parameters (int, bool, object) are passed
		// through correctly instead of being coerced to a string. Interpolated strings and ternary
		// expressions fall back to string resolution below.
		if typed, ok := resolveTypedExpression(val, typedLookup); ok {
			return typed, false, nil
		}
		resolved, err := resolveString(val, lookup, secretLookup)
		if err != nil {
			return nil, false, err
		}
		return resolved, false, nil
	case map[string]any:
		resolved := make(map[string]any, len(val))
		secure := false
		for k, inner := range val {
			r, s, err := resolveValue(inner, lookup, typedLookup, secretLookup)
			if err != nil {
				return nil, false, err
			}
			resolved[k] = r
			secure = secure || s
		}
		return resolved, secure, nil
	case []any:
		resolved := make([]any, len(val))
		secure := false
		for i, inner := range val {
			r, s, err := resolveValue(inner, lookup, typedLookup, secretLookup)
			if err != nil {
				return nil, false, err
			}
			resolved[i] = r
			secure = secure || s
		}
		return resolved, secure, nil
	default:
		return v, false, nil
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

// resolveString replaces all {{...}} expressions in a string with their resolved values. It returns an
// error if a secret expression is used in interpolation (i.e. embedded in surrounding text), because the
// whole-value secret case is handled by the caller and any secret reaching this path would leak partial
// cleartext into a non-secure value.
func resolveString(s string, lookup map[string]string, secretLookup map[string]string) (string, error) {
	var resolveErr error
	out := expressionPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Strip the surrounding {{ and }}.
		inner := match[2 : len(match)-2]
		key := strings.TrimSpace(inner)

		// A secret reaching the interpolation path is embedded in surrounding text (the whole-value
		// case is handled in resolveValue). Reject it so partial cleartext cannot leak.
		if _, isSecret := secretLookup[key]; isSecret {
			resolveErr = fmt.Errorf("secret expression %q may only be used as the entire parameter value, not interpolated into a string", key)
			return match
		}

		// Try ternary evaluation first.
		if result, ok := evaluateTernary(inner, lookup); ok {
			return result
		}

		// Simple context path lookup.
		if val, ok := lookup[key]; ok {
			return val
		}

		// Unrecognized expression — leave unchanged.
		return match
	})
	if resolveErr != nil {
		return "", resolveErr
	}
	return out, nil
}

// evaluateTernary evaluates a ternary expression of the form:
//
//	<context path> == "val" ? <arm> : <arm>
//
// where each arm is either a "string literal" or another (nested) ternary, so chained expressions
// like `a == "S" ? "x" : a == "M" ? "y" : "z"` resolve correctly. It returns the resolved result and
// true when inner is a structurally valid ternary, or ("", false) otherwise. If a condition path
// along the chosen branch cannot be resolved, the entire expression is left unchanged.
//
// Limitations (by design): the only supported operator is ==, arms must be string literals or nested
// ternaries (no context paths or typed results in arms), and unresolvable expressions are passed
// through verbatim rather than failing.
func evaluateTernary(inner string, lookup map[string]string) (string, bool) {
	value, matched, resolved := evalTernaryExpr(inner, lookup)
	if !matched {
		return "", false
	}
	if !resolved {
		// Unresolvable condition — leave the entire expression unchanged.
		return fmt.Sprintf("{{%s}}", inner), true
	}
	return value, true
}

// evalTernaryExpr recursively evaluates a ternary expression. matched reports whether expr is a
// structurally valid ternary; resolved reports whether every condition along the chosen branch was
// found in the lookup. When matched is true but resolved is false, the caller passes the original
// expression through unchanged.
func evalTernaryExpr(expr string, lookup map[string]string) (value string, matched bool, resolved bool) {
	qIdx, colonIdx, ok := splitTopLevelTernary(expr)
	if !ok {
		return "", false, false
	}

	cm := conditionPattern.FindStringSubmatch(strings.TrimSpace(expr[:qIdx]))
	if cm == nil {
		return "", false, false
	}
	conditionPath := strings.TrimSpace(cm[1])
	expectedValue := cm[2]

	conditionValue, found := lookup[conditionPath]
	if !found {
		return "", true, false
	}

	arm := strings.TrimSpace(expr[colonIdx+1:])
	if conditionValue == expectedValue {
		arm = strings.TrimSpace(expr[qIdx+1 : colonIdx])
	}
	return evalTernaryArm(arm, lookup)
}

// evalTernaryArm evaluates a single ternary arm, which is either a nested ternary or a "string literal".
func evalTernaryArm(arm string, lookup map[string]string) (value string, matched bool, resolved bool) {
	if v, m, r := evalTernaryExpr(arm, lookup); m {
		return v, true, r
	}
	if len(arm) >= 2 && strings.HasPrefix(arm, `"`) && strings.HasSuffix(arm, `"`) {
		return arm[1 : len(arm)-1], true, true
	}
	// Unsupported arm (string-only limitation) — pass the expression through unchanged.
	return "", true, false
}

// splitTopLevelTernary locates the top-level "?" and its matching ":" in a ternary expression,
// scanning outside double-quoted string literals and tracking nested ternary depth so that chained
// or nested ternaries split at the outermost level. It returns the byte indices of the "?" and ":"
// and true when both are found.
func splitTopLevelTernary(s string) (qIdx, colonIdx int, ok bool) {
	qIdx, colonIdx = -1, -1
	inQuote := false
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			inQuote = !inQuote
		case '?':
			if inQuote {
				continue
			}
			if depth == 0 && qIdx == -1 {
				qIdx = i
			}
			depth++
		case ':':
			if inQuote {
				continue
			}
			if depth == 0 {
				// ":" with no open "?" — not a valid ternary structure.
				return -1, -1, false
			}
			depth--
			if depth == 0 && colonIdx == -1 {
				colonIdx = i
			}
		}
	}
	if qIdx == -1 || colonIdx == -1 {
		return -1, -1, false
	}
	return qIdx, colonIdx, true
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

// buildSecretLookup builds a flat key-value map of secret material exposed by secret-typed connected
// resources. Keys use the path context.resource.connections.<name>.secrets.<key>. This lookup is kept
// separate from the non-secret context lookup so secret values can only be resolved through the
// whole-value secret path and are tagged for secure routing, never resolved as ordinary string values.
func buildSecretLookup(ctx *recipecontext.Context) map[string]string {
	secrets := map[string]string{}
	if ctx == nil {
		return secrets
	}

	for connName, conn := range ctx.Resource.Connections {
		for key, val := range conn.Secrets {
			secrets[fmt.Sprintf("context.resource.connections.%s.secrets.%s", connName, key)] = val
		}
	}

	return secrets
}
