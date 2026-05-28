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
	"unicode"

	"github.com/mattn/go-runewidth"
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
		if w := runewidth.StringWidth(h) + TablePadSize; w > slots[i] {
			slots[i] = w
		}
	}
	for _, cells := range cellLines {
		for i, lines := range cells {
			for _, line := range lines {
				if w := runewidth.StringWidth(line) + TablePadSize; w > slots[i] {
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

// padRight returns s padded with trailing spaces so that the resulting string occupies
// at least width terminal columns. Width is measured in display columns (so wide
// characters like CJK or emoji count as 2 and combining marks count as 0), so the next
// column starts at the correct visual position. If s is already wider than width, s is
// returned unchanged.
func padRight(s string, width int) string {
	n := runewidth.StringWidth(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

// wordWrap wraps s into lines whose display width is no more than width terminal
// columns, breaking on whitespace where possible. Width is measured in display
// columns (wide characters count as 2, combining marks as 0), so the result fits
// the terminal even when the content contains CJK characters or emoji. Words wider
// than width are broken at rune boundaries (never inside a UTF-8 sequence). Internal
// whitespace runs are preserved within a line; trailing whitespace at a wrap boundary
// is dropped. An empty input returns a single empty line.
func wordWrap(s string, width int) []string {
	if width <= 0 || runewidth.StringWidth(s) <= width {
		return []string{s}
	}

	runes := []rune(s)
	var lines []string
	var current []rune
	currentWidth := 0
	i := 0
	for i < len(runes) {
		// Collect the next non-whitespace word.
		j := i
		for j < len(runes) && !unicode.IsSpace(runes[j]) {
			j++
		}
		word := runes[i:j]
		wordWidth := runewidth.StringWidth(string(word))

		// Collect the whitespace that follows the word.
		k := j
		for k < len(runes) && unicode.IsSpace(runes[k]) {
			k++
		}
		space := runes[j:k]
		spaceWidth := runewidth.StringWidth(string(space))
		i = k

		// Place the word, breaking it across lines if it is itself wider than width.
		if wordWidth > width {
			if len(current) > 0 {
				lines = append(lines, string(trimTrailingSpace(current)))
				current = current[:0]
			}
			// Walk the word and emit chunks of <= width display columns.
			chunkStart := 0
			chunkWidth := 0
			for ci, r := range word {
				rw := runewidth.RuneWidth(r)
				if chunkWidth+rw > width && ci > chunkStart {
					lines = append(lines, string(word[chunkStart:ci]))
					chunkStart = ci
					chunkWidth = 0
				}
				chunkWidth += rw
			}
			current = append(current, word[chunkStart:]...)
			currentWidth = chunkWidth
		} else if currentWidth+wordWidth <= width {
			current = append(current, word...)
			currentWidth += wordWidth
		} else {
			lines = append(lines, string(trimTrailingSpace(current)))
			current = append(current[:0], word...)
			currentWidth = wordWidth
		}

		// Append the trailing whitespace if it still fits on the current line; otherwise
		// drop it at the wrap boundary so we don't emit lines with trailing spaces.
		if len(space) > 0 {
			if currentWidth+spaceWidth <= width {
				current = append(current, space...)
				currentWidth += spaceWidth
			} else {
				lines = append(lines, string(trimTrailingSpace(current)))
				current = current[:0]
				currentWidth = 0
			}
		}
	}
	if len(current) > 0 || len(lines) == 0 {
		lines = append(lines, string(trimTrailingSpace(current)))
	}
	return lines
}

// trimTrailingSpace returns runes with any trailing Unicode whitespace removed.
func trimTrailingSpace(runes []rune) []rune {
	end := len(runes)
	for end > 0 && unicode.IsSpace(runes[end-1]) {
		end--
	}
	return runes[:end]
}

var _ Formatter = (*TableFormatter)(nil)
