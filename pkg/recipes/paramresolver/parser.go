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
)

const (
	// maxExpressionLength bounds the size of a single {{...}} expression.
	maxExpressionLength = 4096

	// maxNestingDepth bounds ternary, unary, and parenthesis nesting so that hostile input cannot
	// exhaust the stack during parsing or evaluation.
	maxNestingDepth = 64
)

// node is an expression AST node.
type node interface {
	isNode()
}

// literalNode is a string, number, boolean, or null literal.
type literalNode struct {
	value any
}

// pathNode is a dotted context path such as context.resource.properties.size.
type pathNode struct {
	path string
}

// notNode is a logical negation.
type notNode struct {
	operand node
}

// binaryNode is an ==, !=, &&, or || operation.
type binaryNode struct {
	op    tokenKind
	left  node
	right node
}

// ternaryNode is a conditional expression: cond ? ifTrue : ifFalse.
type ternaryNode struct {
	cond    node
	ifTrue  node
	ifFalse node
}

func (literalNode) isNode() {}
func (pathNode) isNode()    {}
func (notNode) isNode()     {}
func (binaryNode) isNode()  {}
func (ternaryNode) isNode() {}

// parseExpression parses src into an AST. The grammar, from lowest to highest precedence, is:
//
//	ternary  := or ( "?" ternary ":" ternary )?
//	or       := and ( "||" and )*
//	and      := equality ( "&&" equality )*
//	equality := unary ( ( "==" | "!=" ) unary )*
//	unary    := "!" unary | primary
//	primary  := STRING | NUMBER | "true" | "false" | "null" | PATH | "(" ternary ")"
//
// The whole expression is parsed before evaluation so that syntax errors are reported even in
// branches that would not be evaluated.
func parseExpression(src string) (node, error) {
	if len(src) > maxExpressionLength {
		return nil, fmt.Errorf("expression exceeds the maximum length of %d characters", maxExpressionLength)
	}

	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}

	p := &parser{tokens: tokens}
	if p.peek().kind == tokEOF {
		return nil, fmt.Errorf("empty expression")
	}

	n, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if tok := p.peek(); tok.kind != tokEOF {
		return nil, fmt.Errorf("unexpected %s at position %d", tok.describe(), tok.pos)
	}
	return n, nil
}

type parser struct {
	tokens []token
	pos    int
	depth  int
}

func (p *parser) peek() token {
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	tok := p.tokens[p.pos]
	if tok.kind != tokEOF {
		p.pos++
	}
	return tok
}

func (p *parser) expect(kind tokenKind, want string) error {
	tok := p.next()
	if tok.kind != kind {
		return fmt.Errorf("expected %q but found %s at position %d", want, tok.describe(), tok.pos)
	}
	return nil
}

func (p *parser) enter() error {
	p.depth++
	if p.depth > maxNestingDepth {
		return fmt.Errorf("expression exceeds the maximum nesting depth of %d", maxNestingDepth)
	}
	return nil
}

func (p *parser) leave() {
	p.depth--
}

func (p *parser) parseTernary() (node, error) {
	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokQuestion {
		return cond, nil
	}
	p.next()

	ifTrue, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if err := p.expect(tokColon, ":"); err != nil {
		return nil, err
	}
	ifFalse, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	return ternaryNode{cond: cond, ifTrue: ifTrue, ifFalse: ifFalse}, nil
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: tokOr, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: tokAnd, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseEquality() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for kind := p.peek().kind; kind == tokEq || kind == tokNeq; kind = p.peek().kind {
		p.next()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binaryNode{op: kind, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (node, error) {
	if p.peek().kind != tokNot {
		return p.parsePrimary()
	}
	p.next()

	if err := p.enter(); err != nil {
		return nil, err
	}
	defer p.leave()

	operand, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	return notNode{operand: operand}, nil
}

func (p *parser) parsePrimary() (node, error) {
	tok := p.next()
	switch tok.kind {
	case tokString, tokNumber, tokTrue, tokFalse:
		return literalNode{value: tok.value}, nil
	case tokNull:
		return literalNode{value: nil}, nil
	case tokPath:
		return pathNode{path: tok.text}, nil
	case tokLParen:
		inner, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		if err := p.expect(tokRParen, ")"); err != nil {
			return nil, err
		}
		return inner, nil
	default:
		return nil, fmt.Errorf("expected a value but found %s at position %d", tok.describe(), tok.pos)
	}
}
