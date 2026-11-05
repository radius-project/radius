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
	"math"
	"strconv"
	"strings"
)

// tokenKind identifies the lexical class of a token.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokString
	tokNumber
	tokTrue
	tokFalse
	tokNull
	tokPath
	tokEq
	tokNeq
	tokAnd
	tokOr
	tokNot
	tokQuestion
	tokColon
	tokLParen
	tokRParen
)

// contextRoot is the only identifier that may start a path.
const contextRoot = "context"

// token is a single lexical token. value holds the decoded literal for string and number tokens and
// the dotted path for path tokens. pos is the byte offset of the token in the source expression.
type token struct {
	kind  tokenKind
	text  string
	value any
	pos   int
}

func (t token) describe() string {
	if t.kind == tokEOF {
		return "end of expression"
	}
	return fmt.Sprintf("%q", t.text)
}

// lex splits an expression into tokens. The token set is closed: context paths, string, number,
// boolean, and null literals, and the operators == != && || ! ? : ( ).
func lex(src string) ([]token, error) {
	var tokens []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '"':
			tok, next, err := lexString(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
		case isDigit(c) || (c == '-' && i+1 < len(src) && isDigit(src[i+1])):
			tok, next, err := lexNumber(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
		case isLetter(c):
			tok, next, err := lexWord(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i = next
		default:
			tok, err := lexOperator(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tok)
			i += len(tok.text)
		}
	}
	tokens = append(tokens, token{kind: tokEOF, pos: len(src)})
	return tokens, nil
}

func lexOperator(src string, i int) (token, error) {
	two := ""
	if i+1 < len(src) {
		two = src[i : i+2]
	}
	switch two {
	case "==":
		return token{kind: tokEq, text: two, pos: i}, nil
	case "!=":
		return token{kind: tokNeq, text: two, pos: i}, nil
	case "&&":
		return token{kind: tokAnd, text: two, pos: i}, nil
	case "||":
		return token{kind: tokOr, text: two, pos: i}, nil
	}

	one := src[i : i+1]
	switch one {
	case "!":
		return token{kind: tokNot, text: one, pos: i}, nil
	case "?":
		return token{kind: tokQuestion, text: one, pos: i}, nil
	case ":":
		return token{kind: tokColon, text: one, pos: i}, nil
	case "(":
		return token{kind: tokLParen, text: one, pos: i}, nil
	case ")":
		return token{kind: tokRParen, text: one, pos: i}, nil
	}
	return token{}, fmt.Errorf("unexpected character %q at position %d", src[i], i)
}

// lexString reads a double-quoted string literal starting at src[start]. The literal is the text up
// to the next double quote, taken verbatim: there are no escape sequences, so values such as
// "C:\data" are preserved exactly.
func lexString(src string, start int) (token, int, error) {
	end := strings.IndexByte(src[start+1:], '"')
	if end < 0 {
		return token{}, 0, fmt.Errorf("unterminated string literal at position %d", start)
	}
	end += start + 1
	return token{kind: tokString, text: src[start : end+1], value: src[start+1 : end], pos: start}, end + 1, nil
}

// lexNumber reads an integer or decimal literal with an optional leading minus sign. Integers are
// decoded as int64, or as uint64 when they exceed the int64 range, so they are never rounded. Integers
// outside both ranges are rejected. Decimals are decoded as float64.
func lexNumber(src string, start int) (token, int, error) {
	i := start
	if src[i] == '-' {
		i++
	}
	for i < len(src) && isDigit(src[i]) {
		i++
	}
	isDecimal := false
	if i < len(src) && src[i] == '.' {
		if i+1 >= len(src) || !isDigit(src[i+1]) {
			return token{}, 0, fmt.Errorf("invalid number literal at position %d", start)
		}
		isDecimal = true
		i++
		for i < len(src) && isDigit(src[i]) {
			i++
		}
	}
	if i < len(src) && (isIdentChar(src[i]) || src[i] == '.') {
		return token{}, 0, fmt.Errorf("invalid number literal at position %d", start)
	}

	text := src[start:i]
	if !isDecimal {
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return token{kind: tokNumber, text: text, value: n, pos: start}, i, nil
		}
		if n, err := strconv.ParseUint(text, 10, 64); err == nil {
			return token{kind: tokNumber, text: text, value: n, pos: start}, i, nil
		}
		return token{}, 0, fmt.Errorf("integer literal %q at position %d is out of range: integers must be between %d and %d", text, start, int64(math.MinInt64), uint64(math.MaxUint64))
	}
	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return token{}, 0, fmt.Errorf("invalid number literal %q at position %d", text, start)
	}
	return token{kind: tokNumber, text: text, value: f, pos: start}, i, nil
}

// lexWord reads a keyword (true, false, null) or a dotted context path such as
// context.resource.properties.size. Path segments may contain any character except whitespace,
// double quotes, backticks, brackets, ".", and operator characters, so property keys such as "data/path" can be
// referenced.
func lexWord(src string, start int) (token, int, error) {
	i := start
	for {
		segStart := i
		for i < len(src) && isPathChar(src[i]) {
			i++
		}
		if i == segStart {
			return token{}, 0, fmt.Errorf("invalid path %q at position %d: empty path segment", src[start:i], start)
		}
		if i < len(src) && src[i] == '.' {
			i++
			continue
		}
		break
	}

	text := src[start:i]
	switch text {
	case "true":
		return token{kind: tokTrue, text: text, value: true, pos: start}, i, nil
	case "false":
		return token{kind: tokFalse, text: text, value: false, pos: start}, i, nil
	case "null":
		return token{kind: tokNull, text: text, pos: start}, i, nil
	}
	if !strings.HasPrefix(text, contextRoot+".") {
		return token{}, 0, fmt.Errorf("unsupported identifier %q at position %d: paths must start with %q", text, start, contextRoot+".")
	}
	return token{kind: tokPath, text: text, value: text, pos: start}, i, nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isLetter(c) || isDigit(c) || c == '-'
}

// isPathChar reports whether c may appear in a path segment.
func isPathChar(c byte) bool {
	return !strings.ContainsRune(pathDelimiters, rune(c))
}

// pathDelimiters lists the characters that end a path segment.
const pathDelimiters = " \t\n\r.\"`(){}[]!=&|?:<>+*%,"
