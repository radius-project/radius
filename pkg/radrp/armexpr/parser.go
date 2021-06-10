// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armexpr

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

func IsARMExpression(text string) (bool, error) {
	if !utf8.ValidString(text) {
		return false, errors.New("input is not valid utf8")
	}

	if strings.HasPrefix(text, "[[") && strings.HasSuffix(text, "]") {
		return true, nil
	} else if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		return true, nil
	}

	return false, nil
}

// Parse parses an ARM expression from a string.
func Parse(text string) (*SyntaxTree, error) {
	// The input string is expected to use either the form:
	//
	// [reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).bindings.web]'
	// OR
	// '[[reference(resourceId('Microsoft.CustomProviders/resourceProviders/Applications/Components', 'radius', 'app', 'backend')).bindings.web]'
	//
	// That is, we parse ARM's expression syntax, but also accecpt it with an extra '[' in the front. This allows us escape expresssions and pass
	// them through the deployment engine.
	t := tokenizer{
		Text: text,
	}

	// We validate upfront that the input is valid, since we only operate on runes
	// we don't create invalid cases by incorrect slicing. Therefore we don't need
	// to handle RuneError throughout.
	if !utf8.ValidString(text) {
		return nil, errors.New("input is not valid utf8")
	}

	start := 0
	if strings.HasPrefix(text, "[[") && strings.HasSuffix(text, "]") {
		t.Advance(2)
	} else if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		t.Advance(1)
	} else {
		return nil, errors.New("input is not an expression")
	}

	expression, err := ParseExpression(&t)
	if err != nil {
		return nil, err
	}

	err = t.Expect(']')
	if err != nil {
		return nil, err
	}

	if t.Current < len(t.Text) {
		return nil, errors.New("input contained trailing text after ']'")
	}

	return &SyntaxTree{
		Span: Span{
			Start:  start,
			Length: t.Current - start,
		},
		Expression: expression,
	}, nil
}

func ParseExpression(t *tokenizer) (ExpressionNode, error) {
	var v ExpressionNode
	var err error

	t.SkipWhitespace()
	r, length := t.Peek()

	if r == utf8.RuneError && length == 0 {
		return nil, errors.New("expected expression")
	} else if r == '\'' {
		v, err = ParseString(t)
		if err != nil {
			return nil, err
		}
	} else if unicode.IsLetter(r) {
		v, err = ParsePrimaryExpression(t)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("expected expression")
	}

	for {
		t.SkipWhitespace()
		r, length = t.Peek()

		if r == utf8.RuneError && length == 0 || r != '.' {
			// we've reached the end, return what we have.
			break
		}

		v, err = ParsePropertyAccess(t, v)
		if err != nil {
			return nil, err
		}
	}

	return v, nil
}

func ParsePrimaryExpression(t *tokenizer) (ExpressionNode, error) {
	t.SkipWhitespace()

	identifier, err := ParseIdentifier(t)
	if err != nil {
		return nil, err
	}

	t.SkipWhitespace()

	r, length := t.Peek()
	if r == utf8.RuneError && length == 0 {
		return nil, errors.New("expected expression")
	} else if r == '(' {
		f, err := ParseFunctionCall(t, *identifier)
		if err != nil {
			return nil, err
		}

		return f, nil
	} else {
		return nil, errors.New("expected expression")
	}
}

func ParseFunctionCall(t *tokenizer, identifier IdentifierNode) (ExpressionNode, error) {
	args, err := parseArgumentList(t)
	if err != nil {
		return nil, err
	}

	return &FunctionCallNode{
		Span: Span{
			Start:  identifier.Span.Start,
			Length: t.Current - identifier.Span.Start,
		},
		Identifier: identifier,
		Args:       args,
	}, nil
}

func parseArgumentList(t *tokenizer) ([]ExpressionNode, error) {
	values := []ExpressionNode{}

	err := t.Expect('(')
	if err != nil {
		return nil, err
	}

	t.SkipWhitespace()

	r, length := t.Peek()
	if r == utf8.RuneError && length == 0 {
		return nil, errors.New("expected expression")
	} else if r == ')' {
		// end of argument list
		err = t.Expect(')')
		if err != nil {
			return nil, err
		}

		return values, nil
	}

	// OK we found some arguments
	for {
		t.SkipWhitespace()

		value, err := ParseExpression(t)
		if err != nil {
			return nil, err
		}
		values = append(values, value)

		t.SkipWhitespace()

		r, length = t.Peek()
		if r == utf8.RuneError && length == 0 {
			return nil, errors.New("expected ','")
		} else if r == ',' {
			// Continues the argument list, allow loop to continue
			t.Advance(length)
		} else {
			// Not something that continues the argument list
			break
		}
	}

	err = t.Expect(')')
	if err != nil {
		return nil, err
	}

	return values, nil
}

func ParseString(t *tokenizer) (ExpressionNode, error) {
	start := t.Current

	err := t.Expect('\'')
	if err != nil {
		return nil, err
	}

	var escaping bool = false
	for {
		r, length := t.Peek()
		if r == utf8.RuneError && length == 0 {
			break
		} else if r == '\'' && !escaping {
			break
		} else if r == '\\' {
			// Toggle whether or not it's an escape sequence
			escaping = !escaping
		} else {
			escaping = false
		}

		t.Advance(length)
	}

	if escaping {
		return nil, errors.New("invalid string encountered")
	}

	err = t.Expect('\'')
	if err != nil {
		return nil, err
	}

	return &StringLiteralNode{
		Span: Span{
			Start:  start,
			Length: t.Current - start,
		},
		Text: t.Text[start:t.Current],
	}, nil
}

func ParsePropertyAccess(t *tokenizer, base ExpressionNode) (*PropertyAccessNode, error) {
	// Use the start of the base expression as the start of this node since
	// this is right-associative.
	//
	// eg:
	// ^foo()|.bar
	//
	// If the cursor is | then technically the start of the node is ^
	start := base.GetSpan().Start
	err := t.Expect('.')
	if err != nil {
		return nil, err
	}

	t.SkipWhitespace()

	identifier, err := ParseIdentifier(t)
	if err != nil {
		return nil, err
	}

	return &PropertyAccessNode{
		Span: Span{
			Start:  start,
			Length: t.Current - start,
		},
		Base:       base,
		Identifier: *identifier,
	}, nil
}

func ParseIdentifier(t *tokenizer) (*IdentifierNode, error) {
	t.SkipWhitespace()

	start := t.Current
	for {
		r, length := t.Peek()
		if r == utf8.RuneError && length == 0 {
			break
		} else if !unicode.IsLetter(r) {
			break
		}

		t.Advance(length)
	}

	if start == t.Current {
		return nil, errors.New("identifier expected")
	}

	return &IdentifierNode{
		Span: Span{
			Start:  start,
			Length: t.Current - start,
		},
		Text: t.Text[start:t.Current],
	}, nil
}
