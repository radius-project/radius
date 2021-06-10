// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package armexpr

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

type tokenizer struct {
	Text    string
	Current int
}

func (t *tokenizer) Advance(length int) {
	t.Current = t.Current + length
}

func (t *tokenizer) Peek() (rune, int) {
	return utf8.DecodeRuneInString(t.Text[t.Current:])
}

func (t *tokenizer) Expect(c rune) error {
	r, length := t.Peek()
	if r != c {
		return fmt.Errorf("unexpected token %c, expected %c", r, c)
	}

	t.Advance(length)
	return nil
}

func (t *tokenizer) SkipWhitespace() {
	for {
		r, length := t.Peek()
		if !unicode.IsSpace(r) {
			break
		}

		t.Advance(length)
	}
}
