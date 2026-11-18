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
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/radius-project/radius/pkg/recipes/recipecontext"
)

const (
	openDelimiter  = "{{"
	closeDelimiter = "}}"
)

// ownedExpressionPattern detects a {{...}} body that references a context path. Only such bodies are
// evaluated by the resolver; any other {{...}} text (for example a Helm or Go template such as
// `{{ .Values.name }}`) is passed through unchanged. A reference counts unless it is part of a longer
// name: it must not be preceded by an identifier character, "." (as in `.Values.context.x`), or "$"
// (as in the Go template variable `$context.x`). This means malformed expressions such as
// `{{[context.x]}}` are reported as errors rather than passed through.
var ownedExpressionPattern = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.$])context\.`)

// ResolutionError reports parameter expressions that could not be resolved.
type ResolutionError struct {
	// Errors describes each unresolved expression, ordered by parameter name.
	Errors []error
}

// Error joins the individual expression errors into a single line.
func (e *ResolutionError) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		msgs[i] = err.Error()
	}
	return "failed to resolve recipe parameter expressions: " + strings.Join(msgs, "; ")
}

// Unwrap returns the individual expression errors.
func (e *ResolutionError) Unwrap() []error {
	return e.Errors
}

// ResolveParameterExpressions resolves {{context.*}} expressions in recipe parameters against the
// recipe context. It traverses maps and slices recursively.
//
// When a parameter value consists of exactly one expression, the expression's typed result (string,
// number, bool, object, or array) is substituted. Expressions embedded in surrounding text are
// stringified. {{...}} text that does not reference a context path is left unchanged.
//
// Any expression that is malformed, references an unknown context path, or evaluates to null is an
// error. All such errors are returned together as a *ResolutionError and the returned map is nil.
func ResolveParameterExpressions(params map[string]any, ctx *recipecontext.Context) (map[string]any, error) {
	result, err := resolveParameters(params, ctx)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveParameterExpressionsBestEffort behaves like ResolveParameterExpressions, except that
// expressions that cannot be resolved are left unchanged in the result instead of failing. The
// returned *ResolutionError describes the unresolved expressions and is intended for logging. Use this only
// where failing is worse than passing unresolved text through, such as when deleting a recipe.
func ResolveParameterExpressionsBestEffort(params map[string]any, ctx *recipecontext.Context) (map[string]any, error) {
	return resolveParameters(params, ctx)
}

func resolveParameters(params map[string]any, ctx *recipecontext.Context) (map[string]any, error) {
	if params == nil {
		return nil, nil
	}

	r := &resolver{context: newContextResolver(ctx)}
	result := make(map[string]any, len(params))
	for _, k := range sortedKeys(params) {
		result[k] = r.resolveValue(params[k], k)
	}
	if len(r.errs) > 0 {
		return result, &ResolutionError{Errors: r.errs}
	}
	return result, nil
}

// resolver accumulates errors while resolving a parameter tree.
type resolver struct {
	context *contextResolver
	errs    []error
}

// resolveValue resolves expressions in a single value. It handles strings, maps, and slices
// recursively. path identifies the value in error messages.
func (r *resolver) resolveValue(v any, path string) any {
	switch val := v.(type) {
	case string:
		return r.resolveString(val, path)
	case map[string]any:
		resolved := make(map[string]any, len(val))
		for _, k := range sortedKeys(val) {
			resolved[k] = r.resolveValue(val[k], path+"."+k)
		}
		return resolved
	case []any:
		resolved := make([]any, len(val))
		for i, inner := range val {
			resolved[i] = r.resolveValue(inner, path+"["+strconv.Itoa(i)+"]")
		}
		return resolved
	default:
		return v
	}
}

// resolveString resolves every {{...}} expression in s. If s consists of exactly one context
// expression (ignoring surrounding whitespace) whose result is not a string, or that is a bare
// resource or connection property path, the result is returned as is. Otherwise each result is
// stringified into the surrounding text. Unresolvable expressions are recorded as errors and left
// unchanged in the output.
func (r *resolver) resolveString(s string, path string) any {
	var b strings.Builder
	var typed any
	directProperty := false
	segments := 0
	ownedResolved := 0
	onlyWhitespace := true

	rest := s
	for {
		start := strings.Index(rest, openDelimiter)
		if start < 0 {
			break
		}
		literal := rest[:start]
		b.WriteString(literal)
		if strings.TrimSpace(literal) != "" {
			onlyWhitespace = false
		}

		body := rest[start+len(openDelimiter):]
		inner, consumed, owned, terminated := splitExpression(body)
		if !owned && !terminated {
			// An unterminated "{{" that is not a context expression is plain text.
			rest = rest[start:]
			break
		}
		segments++
		raw := rest[start : start+len(openDelimiter)+consumed]
		rest = rest[start+len(openDelimiter)+consumed:]

		if !owned {
			b.WriteString(raw)
			continue
		}

		value, ast, err := r.evaluateExpression(inner, terminated)
		if err != nil {
			r.errs = append(r.errs, fmt.Errorf("parameter %q: unresolved expression %s: %w", path, raw, err))
			b.WriteString(raw)
			continue
		}

		ownedResolved++
		typed = value
		directProperty = isPropertyPath(ast)
		if str, ok := value.(string); ok {
			b.WriteString(str)
		} else {
			b.WriteString(fmt.Sprintf("%v", value))
		}
	}
	b.WriteString(rest)
	if strings.TrimSpace(rest) != "" {
		onlyWhitespace = false
	}

	if segments == 1 && ownedResolved == 1 && onlyWhitespace {
		if _, isString := typed.(string); !isString || directProperty {
			return typed
		}
	}
	return b.String()
}

// evaluateExpression parses and evaluates the body of a single {{...}} expression.
func (r *resolver) evaluateExpression(inner string, terminated bool) (any, node, error) {
	if !terminated {
		return nil, nil, fmt.Errorf("missing closing %q", closeDelimiter)
	}
	ast, err := parseExpression(inner)
	if err != nil {
		return nil, nil, err
	}
	value, err := r.context.evaluate(ast)
	if err != nil {
		return nil, nil, err
	}
	if value == nil {
		return nil, nil, errNullResult
	}
	return value, ast, nil
}

// isPropertyPath reports whether n is a bare resource or connection property path. Such a whole-value
// expression is replaced by the property value without its surrounding whitespace, as it was before
// expressions were evaluated.
func isPropertyPath(n node) bool {
	p, ok := n.(pathNode)
	if !ok {
		return false
	}
	if strings.HasPrefix(p.path, resourcePropertiesPrefix) {
		return true
	}
	rest, ok := strings.CutPrefix(p.path, resourceConnectionsPrefix)
	if !ok {
		return false
	}
	_, field, _ := strings.Cut(rest, ".")
	return strings.HasPrefix(field, "properties.")
}

// splitExpression locates the end of the {{...}} expression whose body (the text after "{{") starts
// at body. It returns the expression's inner text, the number of bytes consumed including the
// closing "}}", whether the expression references a context path (owned), and whether a closing
// "}}" was found.
//
// Context expressions are closed by the first "}}" outside a double-quoted string literal, so string
// literals may contain "}}". Other {{...}} text is closed by the first "}}", matching how it was
// treated before context expressions were evaluated.
func splitExpression(body string) (inner string, consumed int, owned bool, terminated bool) {
	plain := strings.Index(body, closeDelimiter)
	quoted := indexCloseOutsideQuotes(body)

	switch {
	case plain >= 0 && ownsExpression(body[:plain]),
		quoted >= 0 && ownsExpression(body[:quoted]):
		owned = true
	case plain < 0 && quoted < 0:
		owned = ownsExpression(body)
	}

	switch {
	case owned && quoted >= 0:
		return body[:quoted], quoted + len(closeDelimiter), true, true
	case owned && plain >= 0:
		// The expression has an unbalanced quote; let the parser report it.
		return body[:plain], plain + len(closeDelimiter), true, true
	case owned:
		return body, len(body), true, false
	case plain >= 0:
		return body[:plain], plain + len(closeDelimiter), false, true
	default:
		return "", 0, false, false
	}
}

// indexCloseOutsideQuotes returns the index of the first "}}" in s that is not inside a
// double-quoted string literal, or -1 if there is none.
func indexCloseOutsideQuotes(s string) int {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '"':
			inQuote = !inQuote
		case !inQuote && strings.HasPrefix(s[i:], closeDelimiter):
			return i
		}
	}
	return -1
}

// ownsExpression reports whether an expression body references a context path outside of a
// double-quoted or backtick-quoted string, so text such as `{{ printf "context.x" }}` is not claimed.
func ownsExpression(inner string) bool {
	return ownedExpressionPattern.MatchString(stripQuoted(inner))
}

// stripQuoted replaces each double-quoted or backtick-quoted (Go raw) string in s, including an
// unterminated one, with a single space.
func stripQuoted(s string) string {
	if !strings.ContainsAny(s, "\"`") {
		return s
	}
	var b strings.Builder
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '`':
			quote = c
			b.WriteByte(' ')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	return slices.Sorted(maps.Keys(m))
}
