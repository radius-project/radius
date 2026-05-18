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

package output

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/stretchr/testify/require"
)

type tableInput struct {
	Size   string
	IsCool bool
}

var tableInputOptions = FormatterOptions{
	// Note: We're not testing the behavior of JSONPath in detail since we don't implement that, just the E2E.
	Columns: []Column{
		{
			Heading:  "Size",
			JSONPath: "{ .Size }",
		},
		{
			Heading:  "Coolness",
			JSONPath: "{ .IsCool }",
		},
		{
			Heading:  "Unknown",
			JSONPath: "{ .FieldDoesNotExist }",
		},
		{
			Heading:  "Static",
			JSONPath: "Some-Value",
		},
		{
			Heading:     "Lowered",
			JSONPath:    "Some-Value",
			Transformer: &lowercaseTransformer{},
		},
	},
}

type lowercaseTransformer struct {
}

func (l *lowercaseTransformer) Transform(input string) string {
	return strings.ToLower(input)
}

func Test_Table_NoColumns(t *testing.T) {
	obj := struct{}{}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, FormatterOptions{})
	require.Error(t, err)
	require.Equal(t, "no columns were defined, table format is not supported for this command", err.Error())
}

func Test_Table_Scalar(t *testing.T) {
	obj := tableInput{
		Size:   "mega",
		IsCool: true,
	}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, tableInputOptions)
	require.NoError(t, err)

	expected := `Size      Coolness  Unknown   Static      Lowered
mega      true                Some-Value  some-value
`
	require.Equal(t, expected, buffer.String())
}

func Test_Table_Slice(t *testing.T) {
	obj := []any{
		tableInput{
			Size:   "mega",
			IsCool: true,
		},
		tableInput{
			Size:   "medium",
			IsCool: false,
		},
	}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, tableInputOptions)
	require.NoError(t, err)

	expected := `Size      Coolness  Unknown   Static      Lowered
mega      true                Some-Value  some-value
medium    false               Some-Value  some-value
`
	require.Equal(t, expected, buffer.String())
}

func Test_Table_Slice_Multiline(t *testing.T) {
	obj := []any{
		tableInput{
			Size:   "mega\nsuper\ncool",
			IsCool: true,
		},
		tableInput{
			Size:   "medium\nsmoothness",
			IsCool: false,
		},
	}

	formatter := &TableFormatter{}

	buffer := &bytes.Buffer{}
	err := formatter.Format(obj, buffer, tableInputOptions)
	require.NoError(t, err)

	expected := `Size        Coolness  Unknown   Static      Lowered
mega        true                Some-Value  some-value
super                                       
cool                                        
medium      false               Some-Value  some-value
smoothness                                  
`
	require.Equal(t, expected, buffer.String())
}

type wrappableInput struct {
	Name        string
	Description string
}

var wrappableInputOptions = FormatterOptions{
	Columns: []Column{
		{Heading: "NAME", JSONPath: "{ .Name }"},
		{Heading: "DESCRIPTION", JSONPath: "{ .Description }"},
	},
}

func Test_Table_WrapsLastColumnToTerminalWidth(t *testing.T) {
	obj := []any{
		wrappableInput{
			Name:        "size",
			Description: "The size of the PostgreSQL database, non production environments can be (S)mall or (M)edium, production environments can be or (S)mall, (M)edium, (L)arge, or (XL)arge",
		},
		wrappableInput{
			Name:        "host",
			Description: "The host name of the database server",
		},
	}

	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	// Force a narrow terminal width so the long description must wrap.
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 80)
	require.NoError(t, err)

	// The first column slot is max(len("NAME")+pad, minWidth) = max(6, 10) = 10. The remaining
	// 70 columns are available for the description. Expect the long description to wrap onto
	// continuation lines that are padded under the description column.
	expected := "NAME      DESCRIPTION\n" +
		"size      The size of the PostgreSQL database, non production environments can\n" +
		"          be (S)mall or (M)edium, production environments can be or (S)mall,\n" +
		"          (M)edium, (L)arge, or (XL)arge\n" +
		"host      The host name of the database server\n"

	require.Equal(t, expected, buffer.String())
}

func Test_Table_NoWrapWhenTerminalWidthUnknown(t *testing.T) {
	obj := []any{
		wrappableInput{
			Name:        "size",
			Description: "A long description that would otherwise be wrapped if a terminal width was supplied to the formatter",
		},
	}

	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	// Width 0 means "unknown terminal width" -- no wrapping should occur.
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 0)
	require.NoError(t, err)

	expected := "NAME      DESCRIPTION\n" +
		"size      A long description that would otherwise be wrapped if a terminal width was supplied to the formatter\n"
	require.Equal(t, expected, buffer.String())
}

func Test_Table_WrapsLongUnbreakableWord(t *testing.T) {
	obj := []any{
		wrappableInput{
			Name:        "url",
			Description: "https://example.com/very/long/path/that/has/no/spaces/at/all/and/exceeds/the/width",
		},
	}

	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 40)
	require.NoError(t, err)

	// Available for last column: 40 - 10 = 30. The unbreakable URL should be split mid-word
	// at width boundaries; continuation lines should be padded under the description column.
	expected := "NAME      DESCRIPTION\n" +
		"url       https://example.com/very/long/\n" +
		"          path/that/has/no/spaces/at/all\n" +
		"          /and/exceeds/the/width\n"
	require.Equal(t, expected, buffer.String())
}

// Test_Table_AlignsAndWrapsUnicode verifies that column alignment and word wrapping use
// rune counts rather than byte lengths so multi-byte UTF-8 content stays aligned and is
// never split in the middle of a rune.
func Test_Table_AlignsAndWrapsUnicode(t *testing.T) {
	obj := []any{
		wrappableInput{
			// Multi-byte runes in NAME ensure padding uses rune width, not byte length.
			Name: "café",
			// A description that mixes ASCII and multi-byte runes and exceeds width.
			Description: "naïve façade — emoji 🚀🚀🚀 and more text that should wrap nicely",
		},
	}

	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 40)
	require.NoError(t, err)

	// First column slot is max(len("NAME")+pad, minWidth) = 10. Last column has 30
	// display columns available. Width is measured in terminal display columns, so each
	// 🚀 counts as 2 columns. The wrap should occur on whitespace boundaries and never
	// produce invalid UTF-8.
	expected := "NAME      DESCRIPTION\n" +
		"café      naïve façade — emoji 🚀🚀🚀\n" +
		"          and more text that should wrap\n" +
		"          nicely\n"
	require.Equal(t, expected, buffer.String())
	require.True(t, utf8.ValidString(buffer.String()), "output must be valid UTF-8")
}

// Test_Table_PreservesInternalWhitespace verifies that whitespace inside a cell that fits
// on a line is preserved (only trailing whitespace at a wrap boundary is dropped).
func Test_Table_PreservesInternalWhitespace(t *testing.T) {
	obj := []any{
		wrappableInput{
			Name:        "row",
			Description: "a   b   c",
		},
	}
	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 40)
	require.NoError(t, err)

	expected := "NAME      DESCRIPTION\n" +
		"row       a   b   c\n"
	require.Equal(t, expected, buffer.String())
}

// Test_Table_WrapsWideCharactersByDisplayWidth verifies that columns and wrapping use
// terminal display width rather than rune count: wide characters (e.g. CJK ideographs)
// occupy two columns, so the column slot must grow accordingly and wrapping must occur
// before content exceeds the terminal width.
func Test_Table_WrapsWideCharactersByDisplayWidth(t *testing.T) {
	obj := []any{
		wrappableInput{
			// Each CJK ideograph is two columns wide. "日本語" is 6 columns of display,
			// while it is only 3 runes. NAME slot must size to display width.
			Name:        "日本語",
			Description: "これは日本語の説明文です。長い説明文は端末の幅で折り返されます。",
		},
	}

	formatter := &TableFormatter{}
	buffer := &bytes.Buffer{}
	err := formatter.formatWithWidth(obj, buffer, wrappableInputOptions, 40)
	require.NoError(t, err)

	// "日本語" has display width 6. Slot = max(6+2, len("NAME")+2, minWidth) = 10.
	// Remaining width for the description = 40 - 10 = 30 display columns.
	// Each CJK char is 2 wide; the description has no spaces, so it must be split
	// mid-string on rune boundaries at display-column boundaries.
	got := buffer.String()
	require.True(t, utf8.ValidString(got), "output must be valid UTF-8")

	// Verify that no physical line exceeds the terminal width when measured in
	// display columns. This is the contract the reviewer asked us to enforce.
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		require.LessOrEqual(t, runewidth.StringWidth(line), 40,
			"line %q exceeds terminal width", line)
	}
}

func Test_convertToStruct(t *testing.T) {
	aStruct := tableInput{
		Size: "medium",
	}
	inputs := []convertInput{
		{
			Name:    "string",
			Input:   "test",
			Success: false,
		},
		{
			Name:    "nil",
			Input:   nil,
			Success: false,
		},
		{
			Name:    "nil pointer",
			Input:   (*tableInput)(nil),
			Success: false,
		},
		{
			Name:    "struct",
			Input:   aStruct,
			Success: true,
			Expected: []any{
				aStruct,
			},
		},
		{
			Name:    "struct pointer",
			Input:   &aStruct,
			Success: true,
			Expected: []any{
				aStruct,
			},
		},
		{
			Name: "slice",
			Input: []any{
				aStruct, &aStruct, "test", []any{},
			},
			Success: true,
			Expected: []any{
				aStruct, &aStruct, "test", []any{},
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.Name, func(t *testing.T) {
			actual, err := convertToSlice(input.Input)
			if input.Success {
				require.NoError(t, err)
				require.Equal(t, input.Expected, actual)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type convertInput struct {
	Name     string
	Input    any
	Success  bool
	Expected []any
}
