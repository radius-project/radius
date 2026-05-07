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
	"errors"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
	"k8s.io/client-go/util/jsonpath"
)

// Based on https://golang.org/pkg/text/tabwriter/
const (
	TableColumnMinWidth = 10
	TableTabSize        = 4
	TablePadSize        = 2
	TablePadCharacter   = ' '
	TableFlags          = 0
)

// terminalWidthForWriter is overridable for testing.
var terminalWidthForWriter = func(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	width, _, err := term.GetSize(int(f.Fd()))
	if err != nil || width <= 0 {
		return 0
	}
	return width
}

type TableFormatter struct {
}

// Format() takes in an object, a writer and formatting options and writes a table to the writer using the
// formatting options. If no columns are defined, an error is returned.
//
// When the writer is a terminal, content in the last column that would overflow the terminal width is
// word-wrapped onto continuation lines, and earlier columns are padded with spaces on those continuation
// lines so the table stays visually aligned. When the terminal width is unknown (for example, when output
// is piped), no wrapping is performed.
func (f *TableFormatter) Format(obj any, writer io.Writer, options FormatterOptions) error {
	return f.formatWithWidth(obj, writer, options, terminalWidthForWriter(writer))
}

// formatWithWidth renders the table using an explicit terminal width. A width of 0 (or negative)
// disables wrapping based on terminal width, but cells containing literal newlines are still
// expanded into multiple physical rows.
func (f *TableFormatter) formatWithWidth(obj any, writer io.Writer, options FormatterOptions, terminalWidth int) error {
	if len(options.Columns) == 0 {
		return errors.New("no columns were defined, table format is not supported for this command")
	}

	rows, err := convertToSlice(obj)
	if err != nil {
		return err
	}

	headings := []string{}
	parsers := []*jsonpath.JSONPath{}
	transformers := []ColumnTransformer{}
	for _, c := range options.Columns {
		headings = append(headings, c.Heading)

		p := jsonpath.New(c.Heading).AllowMissingKeys(true)
		if err := p.Parse(c.JSONPath); err != nil {
			return err
		}

		parsers = append(parsers, p)
		transformers = append(transformers, c.Transformer)
	}

	// Resolve every cell up-front. cellLines[r][c] is the slice of lines for the cell at
	// row r and column c after splitting any embedded newlines from the source data.
	cellLines := make([][][]string, 0, len(rows))
	for _, row := range rows {
		cells := make([][]string, len(parsers))
		for i, p := range parsers {
			buf := bytes.Buffer{}
			if err := p.Execute(&buf, row); err != nil {
				return err
			}

			text := buf.String()
			if transformers[i] != nil {
				text = transformers[i].Transform(text)
			}

			cells[i] = strings.Split(text, "\n")
		}
		cellLines = append(cellLines, cells)
	}

	// Compute each column's natural slot width: max(content+pad, minWidth). The slot
	// is the total number of characters consumed by the column on every rendered line,
	// so adjacent columns sit flush against each other (no extra separator characters).
	numCols := len(headings)
	slots := make([]int, numCols)
	for i, h := range headings {
		if w := len(h) + TablePadSize; w > slots[i] {
			slots[i] = w
		}
	}
	for _, cells := range cellLines {
		for i, lines := range cells {
			for _, line := range lines {
				if w := len(line) + TablePadSize; w > slots[i] {
					slots[i] = w
				}
			}
		}
	}
	for i := range slots {
		if slots[i] < TableColumnMinWidth {
			slots[i] = TableColumnMinWidth
		}
	}

	// The last column is rendered without trailing padding, so its effective content width
	// is its slot width minus the pad we added when computing the slot. When we know the
	// terminal width and the table would overflow, shrink the last column so its content
	// fits in the remaining space, and wrap longer cells onto continuation lines.
	lastContentWidth := slots[numCols-1] - TablePadSize
	if terminalWidth > 0 {
		nonLastSum := 0
		for i := 0; i < numCols-1; i++ {
			nonLastSum += slots[i]
		}
		available := terminalWidth - nonLastSum
		if available < TableColumnMinWidth {
			available = TableColumnMinWidth
		}
		if available < lastContentWidth {
			lastContentWidth = available
		}
	}

	writeRow := func(cells [][]string) error {
		// Wrap the last column's lines so that each physical line fits within lastContentWidth.
		if numCols > 0 {
			lastIdx := numCols - 1
			wrapped := make([]string, 0, len(cells[lastIdx]))
			for _, line := range cells[lastIdx] {
				wrapped = append(wrapped, wordWrap(line, lastContentWidth)...)
			}
			cells[lastIdx] = wrapped
		}

		// Compute row height (max number of physical lines across all columns).
		height := 1
		for _, c := range cells {
			if len(c) > height {
				height = len(c)
			}
		}

		for li := 0; li < height; li++ {
			for ci, c := range cells {
				cell := ""
				if li < len(c) {
					cell = c[li]
				}
				if ci < numCols-1 {
					// Pad non-last cells (including empty cells on continuation lines)
					// to the full slot width so columns stay aligned.
					if _, err := io.WriteString(writer, padRight(cell, slots[ci])); err != nil {
						return err
					}
				} else {
					// Last column: write content as-is, no trailing padding.
					if _, err := io.WriteString(writer, cell); err != nil {
						return err
					}
				}
			}
			if _, err := io.WriteString(writer, "\n"); err != nil {
				return err
			}
		}
		return nil
	}

	// Heading row.
	headingCells := make([][]string, numCols)
	for i, h := range headings {
		headingCells[i] = []string{h}
	}
	if err := writeRow(headingCells); err != nil {
		return err
	}

	for _, cells := range cellLines {
		if err := writeRow(cells); err != nil {
			return err
		}
	}

	return nil
}

// padRight returns s padded with trailing spaces so that the resulting string is at least
// width characters long. If s is already wider than width, s is returned unchanged.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// wordWrap wraps s into lines no wider than width characters, breaking on whitespace where
// possible. Words longer than width are broken mid-word. An empty input returns a single
// empty line so that callers can iterate the result without special-casing empty cells.
func wordWrap(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	if len(s) <= width {
		return []string{s}
	}

	var lines []string
	current := ""
	for _, word := range strings.Fields(s) {
		// Break words that are themselves longer than the column width.
		for len(word) > width {
			if current != "" {
				lines = append(lines, current)
				current = ""
			}
			lines = append(lines, word[:width])
			word = word[width:]
		}
		switch {
		case current == "":
			current = word
		case len(current)+1+len(word) <= width:
			current += " " + word
		default:
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

var _ Formatter = (*TableFormatter)(nil)
