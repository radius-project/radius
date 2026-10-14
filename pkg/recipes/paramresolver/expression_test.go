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
	"errors"
	"strings"
	"testing"

	"github.com/radius-project/radius/pkg/recipes/recipecontext"
	"github.com/stretchr/testify/require"
)

// expressionContext extends testContext with properties used by the expression tests.
func expressionContext() *recipecontext.Context {
	ctx := testContext()
	ctx.Resource.Properties["size"] = "M"
	ctx.Resource.Properties["ratio"] = 0.5
	ctx.Resource.Properties["replicas"] = float64(3)
	ctx.Resource.Properties["empty"] = ""
	ctx.Resource.Properties["disabled"] = false
	ctx.Resource.Properties["tags"] = map[string]any{"team": "data"}
	ctx.Resource.Properties["zones"] = []any{"1", "2"}
	ctx.Resource.Properties["quote"] = `say "hi" }}`
	return ctx
}

// resolveOne resolves a single parameter value against expressionContext.
func resolveOne(t *testing.T, value string) (any, error) {
	t.Helper()
	result, err := ResolveParameterExpressions(map[string]any{"p": value}, expressionContext())
	if err != nil {
		return nil, err
	}
	return result["p"], nil
}

func Test_Expressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected any
	}{
		// Nesting and chaining.
		{name: "chained ternary first", expr: `{{context.resource.properties.size == "S" ? "small" : context.resource.properties.size == "M" ? "medium" : "large"}}`, expected: "medium"},
		{name: "nested ternary in true arm", expr: `{{context.resource.properties.size == "M" ? (context.resource.properties.enabled ? "m-on" : "m-off") : "other"}}`, expected: "m-on"},
		{name: "unparenthesized nested ternary in true arm", expr: `{{context.resource.properties.enabled ? context.resource.properties.disabled ? "a" : "b" : "c"}}`, expected: "b"},

		// Operators.
		{name: "not equal true", expr: `{{context.resource.properties.size != "S" ? "yes" : "no"}}`, expected: "yes"},
		{name: "not equal false", expr: `{{context.resource.properties.size != "M" ? "yes" : "no"}}`, expected: "no"},
		{name: "and true", expr: `{{context.resource.properties.size == "M" && context.resource.properties.enabled ? "yes" : "no"}}`, expected: "yes"},
		{name: "and false", expr: `{{context.resource.properties.size == "M" && context.resource.properties.disabled ? "yes" : "no"}}`, expected: "no"},
		{name: "or true", expr: `{{context.resource.properties.size == "S" || context.resource.properties.size == "M" ? "yes" : "no"}}`, expected: "yes"},
		{name: "or false", expr: `{{context.resource.properties.size == "S" || context.resource.properties.size == "L" ? "yes" : "no"}}`, expected: "no"},
		{name: "not", expr: `{{!context.resource.properties.disabled ? "yes" : "no"}}`, expected: "yes"},
		{name: "double not", expr: `{{!!context.resource.properties.enabled}}`, expected: true},
		{name: "and binds tighter than or", expr: `{{context.resource.properties.enabled || false && false}}`, expected: true},
		{name: "parentheses override precedence", expr: `{{(context.resource.properties.enabled || false) && false}}`, expected: false},
		{name: "equality binds tighter than and", expr: `{{context.resource.properties.size == "M" && "a" == "a"}}`, expected: true},
		{name: "not binds tighter than equality", expr: `{{!context.resource.properties.disabled == true}}`, expected: true},
		{name: "and short-circuits unknown path", expr: `{{false && context.unknown.path ? "yes" : "no"}}`, expected: "no"},
		{name: "or short-circuits unknown path", expr: `{{true || context.unknown.path ? "yes" : "no"}}`, expected: "yes"},

		// Operands and typed results.
		{name: "path in arm preserves type", expr: `{{context.resource.properties.enabled ? context.resource.properties.port : 0}}`, expected: 5432},
		{name: "integer literal result", expr: `{{context.resource.properties.size == "L" ? 512 : 128}}`, expected: int64(128)},
		{name: "negative integer literal", expr: `{{context.resource.properties.enabled ? -1 : 0}}`, expected: int64(-1)},
		{name: "decimal literal result", expr: `{{context.resource.properties.enabled ? 1.5 : 2.5}}`, expected: 1.5},
		{name: "bool literal result", expr: `{{context.resource.properties.size == "M" ? true : false}}`, expected: true},
		{name: "comparison result is bool", expr: `{{context.resource.properties.size == "M"}}`, expected: true},
		{name: "object result", expr: `{{context.resource.properties.enabled ? context.resource.properties.tags : context.resource.properties.zones}}`, expected: map[string]any{"team": "data"}},
		{name: "array result", expr: `{{context.resource.properties.disabled ? context.resource.properties.tags : context.resource.properties.zones}}`, expected: []any{"1", "2"}},
		{name: "string arm from context", expr: `{{context.resource.properties.enabled ? context.resource.name : "x"}}`, expected: "my-resource"},
		{name: "surrounding whitespace keeps typed result", expr: `  {{context.resource.properties.port}}  `, expected: 5432},
		{name: "surrounding whitespace kept for string result", expr: ` {{context.resource.name}} `, expected: " my-resource "},

		// Interpolation stringifies.
		{name: "interpolated number", expr: `size-{{context.resource.properties.size == "L" ? 512 : 128}}`, expected: "size-128"},
		{name: "interpolated bool", expr: `enabled={{context.resource.properties.enabled}}`, expected: "enabled=true"},
		{name: "two expressions stringify", expr: `{{context.resource.properties.port}}{{context.resource.properties.enabled}}`, expected: "5432true"},
		{name: "expression plus non-context template stringifies", expr: `{{context.resource.properties.port}}{{ .Values.x }}`, expected: "5432{{ .Values.x }}"},

		// Equality semantics.
		{name: "number equals number across types", expr: `{{context.resource.properties.replicas == 3}}`, expected: true},
		{name: "int equals decimal literal", expr: `{{context.resource.properties.port == 5432.0}}`, expected: true},
		{name: "number equals string form", expr: `{{context.resource.properties.port == "5432"}}`, expected: true},
		{name: "bool equals string form", expr: `{{context.resource.properties.enabled == "true"}}`, expected: true},
		{name: "bool equals bool", expr: `{{context.resource.properties.enabled == true}}`, expected: true},
		{name: "decimal property", expr: `{{context.resource.properties.ratio == 0.5}}`, expected: true},

		// Null handling for absent optional properties.
		{name: "absent property equals null", expr: `{{context.resource.properties.missing == null}}`, expected: true},
		{name: "present property not null", expr: `{{context.resource.properties.size != null}}`, expected: true},
		{name: "absent property is falsy", expr: `{{context.resource.properties.missing ? "set" : "unset"}}`, expected: "unset"},
		{name: "default for absent property", expr: `{{context.resource.properties.missing != null ? context.resource.properties.missing : "default"}}`, expected: "default"},
		{name: "absent connection property is null", expr: `{{context.resource.connections.missing.properties.host == null}}`, expected: true},
		{name: "null equals null", expr: `{{context.resource.properties.missing == null && null == null}}`, expected: true},
		{name: "null not equal to empty string", expr: `{{context.resource.properties.missing == ""}}`, expected: false},

		// Truthiness.
		{name: "empty string is falsy", expr: `{{context.resource.properties.empty ? "a" : "b"}}`, expected: "b"},
		{name: "non-empty string is truthy", expr: `{{context.resource.properties.size ? "a" : "b"}}`, expected: "a"},
		{name: "zero is falsy", expr: `{{context.resource.properties.enabled && 0 ? "a" : "b"}}`, expected: "b"},
		{name: "non-zero number is truthy", expr: `{{context.resource.properties.port ? "a" : "b"}}`, expected: "a"},
		{name: "non-empty object is truthy", expr: `{{context.resource.properties.tags ? "a" : "b"}}`, expected: "a"},

		// Literals and scanning.
		{name: "escaped quote in string literal", expr: `{{context.resource.properties.enabled ? "say \"hi\"" : "no"}}`, expected: `say "hi"`},
		{name: "escaped backslash in string literal", expr: `{{context.resource.properties.enabled ? "a\\b" : "no"}}`, expected: `a\b`},
		{name: "closing braces inside string literal", expr: `{{context.resource.properties.quote == "say \"hi\" }}" ? "match" : "no"}}`, expected: "match"},
		{name: "single brace inside string literal", expr: `x-{{context.resource.properties.enabled ? "a}b" : "c"}}-y`, expected: "x-a}b-y"},
		{name: "hyphenated property name", expr: `{{context.resource.properties.my-prop == null}}`, expected: true},
		{name: "whitespace inside braces", expr: "{{  context.resource.name\t}}", expected: "my-resource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveOne(t, tt.expr)
			require.NoError(t, err)
			require.Equal(t, tt.expected, got)
		})
	}
}

func Test_Expressions_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		expr        string
		expectedErr string
	}{
		{name: "unknown path", expr: `{{context.unknown}}`, expectedErr: `unknown context path "context.unknown"`},
		{name: "null result", expr: `{{context.resource.properties.missing}}`, expectedErr: "expression evaluated to null"},
		{name: "null result from arm", expr: `{{context.resource.properties.enabled ? null : "x"}}`, expectedErr: "expression evaluated to null"},
		{name: "null result interpolated", expr: `x-{{context.resource.properties.missing}}`, expectedErr: "expression evaluated to null"},
		{name: "missing colon", expr: `{{context.resource.properties.enabled ? "a"}}`, expectedErr: `expected ":" but found end of expression`},
		{name: "missing arm", expr: `{{context.resource.properties.enabled ? "a" :}}`, expectedErr: "expected a value but found end of expression"},
		{name: "trailing input", expr: `{{context.resource.name "extra"}}`, expectedErr: `unexpected "\"extra\""`},
		{name: "unbalanced parenthesis", expr: `{{(context.resource.properties.enabled}}`, expectedErr: `expected ")"`},
		{name: "unexpected close parenthesis", expr: `{{context.resource.name)}}`, expectedErr: `unexpected ")"`},
		{name: "single equals", expr: `{{context.resource.name = "x"}}`, expectedErr: `unexpected character '='`},
		{name: "single ampersand", expr: `{{context.resource.properties.enabled & true}}`, expectedErr: `unexpected character '&'`},
		{name: "unsupported identifier", expr: `{{context.resource.properties.enabled ? foo : "x"}}`, expectedErr: `unsupported identifier "foo"`},
		{name: "function call", expr: `{{context.resource.name.toUpper()}}`, expectedErr: `unexpected "("`},
		{name: "unterminated string", expr: `{{context.resource.name == "abc}}`, expectedErr: "unterminated string literal"},
		{name: "unsupported escape", expr: `{{context.resource.name == "a\nb"}}`, expectedErr: "unsupported escape sequence"},
		{name: "invalid number", expr: `{{context.resource.properties.port == 12ab}}`, expectedErr: "invalid number literal"},
		{name: "trailing dot in path", expr: `{{context.resource.name. == "x"}}`, expectedErr: "empty path segment"},
		{name: "unterminated expression", expr: `{{context.resource.name`, expectedErr: `missing closing "}}"`},
		{name: "compare objects", expr: `{{context.resource.properties.tags == "x"}}`, expectedErr: "cannot compare object or array values"},
		{name: "syntax error in arm not taken", expr: `{{context.resource.properties.enabled ? "a" : (}}`, expectedErr: "expected a value"},
		{name: "unknown path on chosen arm", expr: `{{true ? context.unknown : "x"}}`, expectedErr: `unknown context path "context.unknown"`},
		{name: "too long", expr: "{{context.resource.name == \"" + strings.Repeat("a", maxExpressionLength) + "\"}}", expectedErr: "maximum length"},
		{name: "too deep", expr: "{{" + strings.Repeat("(", maxNestingDepth+1) + "context.resource.name" + strings.Repeat(")", maxNestingDepth+1) + "}}", expectedErr: "maximum nesting depth"},
		{name: "too many nots", expr: "{{" + strings.Repeat("!", maxNestingDepth+1) + "context.resource.name}}", expectedErr: "maximum nesting depth"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := resolveOne(t, tt.expr)
			require.ErrorContains(t, err, tt.expectedErr)

			resErr, ok := errors.AsType[*ResolutionError](err)
			require.True(t, ok, "expected a *ResolutionError")
			require.Len(t, resErr.Errors, 1)
			require.ErrorContains(t, err, `parameter "p": unresolved expression `)
		})
	}
}

func Test_ResolveParameterExpressions_ReportsAllErrors(t *testing.T) {
	t.Parallel()

	params := map[string]any{
		"b": "{{context.unknown.b}}",
		"a": map[string]any{
			"list": []any{"ok", "{{context.unknown.list}}"},
		},
		"c": "{{context.resource.name}}",
	}

	result, err := ResolveParameterExpressions(params, testContext())
	require.Nil(t, result)

	resErr, ok := errors.AsType[*ResolutionError](err)
	require.True(t, ok)
	require.Len(t, resErr.Errors, 2)
	require.Equal(t,
		`failed to resolve recipe parameter expressions: `+
			`parameter "a.list[1]": unresolved expression {{context.unknown.list}}: unknown context path "context.unknown.list"; `+
			`parameter "b": unresolved expression {{context.unknown.b}}: unknown context path "context.unknown.b"`,
		err.Error())
}

func Test_ResolveParameterExpressionsBestEffort(t *testing.T) {
	t.Parallel()

	params := map[string]any{
		"bad":    "{{context.unknown.path}}",
		"mixed":  "{{context.resource.name}}-{{context.unknown.path}}",
		"good":   "{{context.resource.properties.port}}",
		"nested": map[string]any{"bad": `{{context.resource.properties.enabled ? "a" :}}`},
	}

	result, err := ResolveParameterExpressionsBestEffort(params, testContext())
	require.Equal(t, map[string]any{
		"bad":    "{{context.unknown.path}}",
		"mixed":  "my-resource-{{context.unknown.path}}",
		"good":   5432,
		"nested": map[string]any{"bad": `{{context.resource.properties.enabled ? "a" :}}`},
	}, result)

	resErr, ok := errors.AsType[*ResolutionError](err)
	require.True(t, ok)
	require.Len(t, resErr.Errors, 3)

	t.Run("no errors", func(t *testing.T) {
		t.Parallel()
		result, err := ResolveParameterExpressionsBestEffort(map[string]any{"name": "{{context.resource.name}}"}, testContext())
		require.NoError(t, err)
		require.Equal(t, map[string]any{"name": "my-resource"}, result)
	})

	t.Run("nil params", func(t *testing.T) {
		t.Parallel()
		result, err := ResolveParameterExpressionsBestEffort(nil, testContext())
		require.NoError(t, err)
		require.Nil(t, result)
	})
}

func Test_lex(t *testing.T) {
	t.Parallel()

	tokens, err := lex(`!(context.resource.properties.size == "S") && x_y != -1.5 || true ? null : false`)
	require.ErrorContains(t, err, `unsupported identifier "x_y"`)
	require.Nil(t, tokens)

	tokens, err = lex(`!(context.resource.properties.size == "S\"") && 42 != -1.5 || true ? null : false`)
	require.NoError(t, err)

	kinds := make([]tokenKind, len(tokens))
	for i, tok := range tokens {
		kinds[i] = tok.kind
	}
	require.Equal(t, []tokenKind{
		tokNot, tokLParen, tokPath, tokEq, tokString, tokRParen, tokAnd, tokNumber, tokNeq, tokNumber,
		tokOr, tokTrue, tokQuestion, tokNull, tokColon, tokFalse, tokEOF,
	}, kinds)
	require.Equal(t, "context.resource.properties.size", tokens[2].value)
	require.Equal(t, `S"`, tokens[4].value)
	require.Equal(t, int64(42), tokens[7].value)
	require.Equal(t, -1.5, tokens[9].value)
}

func Test_parseExpression_EmptyExpression(t *testing.T) {
	t.Parallel()

	_, err := parseExpression("   ")
	require.ErrorContains(t, err, "empty expression")
}
