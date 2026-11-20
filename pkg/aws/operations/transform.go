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

package operations

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// transform is a parsed CloudFormation propertyTransform: a JSONata expression, or several joined by CloudFormation's
// $OR, that turns the template's value into a pattern AWS's value must match. For example
// `$join([$string(EngineVersion), ".*"])` turns "8.4" into a pattern that matches "8.4.11".
//
// Only the functions most schemas use are supported: $lowercase, $uppercase, $join, and $string, which $join often
// wraps. Arguments are string literals, array literals, and property references. A transform with anything else is
// not parsed, so the property is compared as it is.
//
// Every alternative must read a template property. Text that comes from the template is matched literally. String
// literals in the expression are regular expressions, which is how schemas write patterns such as a KMS key ARN prefix.
type transform struct {
	alternatives []node
}

type node interface {
	eval(parent map[string]any, root map[string]any) (value, bool)
}

// errUnsupported marks an expression that uses a function or syntax UCP does not evaluate.
var errUnsupported = errors.New("unsupported property transform")

func parseTransform(expression string) (*transform, error) {
	p := &parser{input: expression}
	t := &transform{}
	for {
		n, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		// An alternative that reads no property, such as a fixed KMS key ARN pattern, would match any value AWS
		// reports in that form, so a real change in the template would never be sent.
		if !readsProperty(n) {
			return nil, fmt.Errorf("%w: an alternative does not read a property", errUnsupported)
		}
		t.alternatives = append(t.alternatives, n)
		p.skipSpace()
		if p.done() {
			return t, nil
		}
		if !p.consumeWord("$OR") {
			return nil, fmt.Errorf("%w: unexpected %q", errUnsupported, p.rest())
		}
	}
}

func readsProperty(n node) bool {
	switch v := n.(type) {
	case pathNode:
		return true
	case listNode:
		return slices.ContainsFunc(v.items, readsProperty)
	case callNode:
		return slices.ContainsFunc(v.args, readsProperty)
	}
	return false
}

// matches reports whether current matches any alternative evaluated against the template's values. parent is the
// object that holds the property, and root is the whole properties object: schemas refer to a nested property both by
// its name and by its full path.
func (t *transform) matches(parent map[string]any, root map[string]any, current any) bool {
	actual, ok := current.(string)
	if !ok {
		return false
	}
	for _, alternative := range t.alternatives {
		v, ok := alternative.eval(parent, root)
		if !ok {
			continue
		}
		p, ok := v.(pattern)
		if !ok {
			continue
		}
		re, err := regexp.Compile("^(?:" + p.regexp() + ")$")
		if err == nil && re.MatchString(actual) {
			return true
		}
	}
	return false
}

// value is the result of evaluating a node: a pattern, a list of values, or a JSON scalar (float64 or bool).
type value any

// pattern is a string made of literal text from the template and regular expression text from the expression.
type pattern []piece

type piece struct {
	text  string
	regex bool
}

func (p pattern) regexp() string {
	var b strings.Builder
	for _, s := range p {
		if s.regex {
			b.WriteString(s.text)
		} else {
			b.WriteString(regexp.QuoteMeta(s.text))
		}
	}
	return b.String()
}

func (p pattern) mapText(f func(string) string) pattern {
	out := make(pattern, len(p))
	for i, s := range p {
		out[i] = piece{text: f(s.text), regex: s.regex}
	}
	return out
}

type stringNode struct{ text string }

func (n stringNode) eval(map[string]any, map[string]any) (value, bool) {
	return pattern{{text: n.text, regex: true}}, true
}

type listNode struct{ items []node }

func (n listNode) eval(parent map[string]any, root map[string]any) (value, bool) {
	out := []value{}
	for _, item := range n.items {
		v, ok := item.eval(parent, root)
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

type pathNode struct{ segments []string }

func (n pathNode) eval(parent map[string]any, root map[string]any) (value, bool) {
	for _, obj := range []map[string]any{parent, root} {
		var current any = obj
		found := true
		for _, s := range n.segments {
			m, ok := current.(map[string]any)
			if !ok {
				found = false
				break
			}
			if current, ok = m[s]; !ok || current == nil {
				found = false
				break
			}
		}
		if found {
			return fromJSON(current)
		}
	}
	return nil, false
}

func fromJSON(v any) (value, bool) {
	switch t := v.(type) {
	case string:
		return pattern{{text: t}}, true
	case float64, bool:
		return t, true
	case []any:
		out := []value{}
		for _, item := range t {
			converted, ok := fromJSON(item)
			if !ok {
				return nil, false
			}
			out = append(out, converted)
		}
		return out, true
	default:
		return nil, false
	}
}

type callNode struct {
	name string
	args []node
}

// supportedFunctions maps each supported function to the number of arguments it accepts.
var supportedFunctions = map[string][2]int{
	"$lowercase": {1, 1},
	"$uppercase": {1, 1},
	"$string":    {1, 1},
	"$join":      {1, 2},
}

func (n callNode) eval(parent map[string]any, root map[string]any) (value, bool) {
	args := make([]value, len(n.args))
	for i, a := range n.args {
		v, ok := a.eval(parent, root)
		if !ok {
			return nil, false
		}
		args[i] = v
	}
	switch n.name {
	case "$lowercase", "$uppercase":
		p, ok := args[0].(pattern)
		if !ok {
			return nil, false
		}
		if n.name == "$lowercase" {
			return p.mapText(strings.ToLower), true
		}
		return p.mapText(strings.ToUpper), true
	case "$string":
		switch v := args[0].(type) {
		case pattern:
			return v, true
		case float64:
			return pattern{{text: strconv.FormatFloat(v, 'f', -1, 64)}}, true
		case bool:
			return pattern{{text: strconv.FormatBool(v)}}, true
		}
		return nil, false
	default:
		// $join, the only other function the parser accepts.
		items, ok := args[0].([]value)
		if !ok {
			// JSONata treats a single string as an array of one.
			p, isPattern := args[0].(pattern)
			if !isPattern {
				return nil, false
			}
			items = []value{p}
		}
		separator := pattern{}
		if len(args) == 2 {
			if separator, ok = args[1].(pattern); !ok {
				return nil, false
			}
		}
		out := pattern{}
		for i, item := range items {
			p, ok := item.(pattern)
			if !ok {
				return nil, false
			}
			if i > 0 {
				out = append(out, separator...)
			}
			out = append(out, p...)
		}
		return out, true
	}
}

// parser reads the supported subset of JSONata.
type parser struct {
	input string
	pos   int
}

func (p *parser) done() bool   { return p.pos >= len(p.input) }
func (p *parser) rest() string { return p.input[p.pos:] }

func (p *parser) skipSpace() {
	for !p.done() && unicode.IsSpace(rune(p.input[p.pos])) {
		p.pos++
	}
}

func (p *parser) peek() byte {
	if p.done() {
		return 0
	}
	return p.input[p.pos]
}

// consumeWord consumes word when it is next and not followed by another name character.
func (p *parser) consumeWord(word string) bool {
	if !strings.HasPrefix(p.rest(), word) {
		return false
	}
	end := p.pos + len(word)
	if end < len(p.input) && isNameChar(p.input[end]) {
		return false
	}
	p.pos = end
	return true
}

func (p *parser) parseExpression() (node, error) {
	p.skipSpace()
	n, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	// Anything other than the end of an argument, a list item, or the expression is an operator, a predicate, or
	// another construct UCP does not evaluate.
	switch c := p.peek(); {
	case c == 0, c == ',', c == ')', c == ']', strings.HasPrefix(p.rest(), "$OR"):
		return n, nil
	default:
		return nil, fmt.Errorf("%w: unexpected %q", errUnsupported, p.rest())
	}
}

func (p *parser) parsePrimary() (node, error) {
	switch c := p.peek(); {
	case c == '"' || c == '\'':
		text, err := p.parseString()
		if err != nil {
			return nil, err
		}
		return stringNode{text: text}, nil
	case c == '[':
		p.pos++
		items, err := p.parseList(']')
		if err != nil {
			return nil, err
		}
		return listNode{items: items}, nil
	case c == '$':
		start := p.pos
		p.pos++
		for !p.done() && isNameChar(p.peek()) {
			p.pos++
		}
		name := p.input[start:p.pos]
		limits, ok := supportedFunctions[name]
		if !ok {
			return nil, fmt.Errorf("%w: function %s", errUnsupported, name)
		}
		p.skipSpace()
		if p.peek() != '(' {
			return nil, fmt.Errorf("%w: %s is not called", errUnsupported, name)
		}
		p.pos++
		args, err := p.parseList(')')
		if err != nil {
			return nil, err
		}
		if len(args) < limits[0] || len(args) > limits[1] {
			return nil, fmt.Errorf("%w: %s takes %d to %d arguments", errUnsupported, name, limits[0], limits[1])
		}
		return callNode{name: name, args: args}, nil
	case isNameStart(c):
		segments := []string{}
		for {
			start := p.pos
			for !p.done() && isNameChar(p.peek()) {
				p.pos++
			}
			segments = append(segments, p.input[start:p.pos])
			if p.peek() != '.' {
				break
			}
			p.pos++
			if !isNameStart(p.peek()) {
				return nil, fmt.Errorf("%w: unexpected %q", errUnsupported, p.rest())
			}
		}
		return pathNode{segments: segments}, nil
	default:
		return nil, fmt.Errorf("%w: unexpected %q", errUnsupported, p.rest())
	}
}

func (p *parser) parseList(end byte) ([]node, error) {
	items := []node{}
	p.skipSpace()
	if p.peek() == end {
		p.pos++
		return items, nil
	}
	for {
		n, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		items = append(items, n)
		switch p.peek() {
		case ',':
			p.pos++
		case end:
			p.pos++
			return items, nil
		default:
			return nil, fmt.Errorf("%w: unexpected %q", errUnsupported, p.rest())
		}
	}
}

// parseString reads a quoted string literal with JSON escapes.
func (p *parser) parseString() (string, error) {
	quote := p.input[p.pos]
	p.pos++
	var b strings.Builder
	for !p.done() {
		c := p.input[p.pos]
		p.pos++
		switch c {
		case quote:
			return b.String(), nil
		case '\\':
			if p.done() {
				return "", fmt.Errorf("%w: unterminated string", errUnsupported)
			}
			e := p.input[p.pos]
			p.pos++
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'u':
				if p.pos+4 > len(p.input) {
					return "", fmt.Errorf("%w: bad escape", errUnsupported)
				}
				r, err := strconv.ParseUint(p.input[p.pos:p.pos+4], 16, 16)
				if err != nil {
					return "", fmt.Errorf("%w: bad escape", errUnsupported)
				}
				b.WriteRune(rune(r))
				p.pos += 4
			default:
				// \" \' \\ \/ and any other escaped character stand for themselves.
				b.WriteByte(e)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", fmt.Errorf("%w: unterminated string", errUnsupported)
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
