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

package show

import (
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/radius-project/radius/pkg/cli/output"
)

// minDescriptionWidth is the narrowest DESCRIPTION column we wrap to. When the terminal is too
// narrow to fit this, the description column still wraps at this width so text stays readable.
const minDescriptionWidth = 20

// TerminalWidthFunc returns the width of the terminal in columns and whether the width is known.
type TerminalWidthFunc func() (int, bool)

// stdoutTerminalWidth returns the width of stdout when it is a terminal.
func stdoutTerminalWidth() (int, bool) {
	fd := os.Stdout.Fd()
	if !term.IsTerminal(fd) {
		return 0, false
	}

	width, _, err := term.GetSize(fd)
	if err != nil || width <= 0 {
		return 0, false
	}

	return width, true
}

// terminalWidth returns the terminal width to wrap output to, or false when output should not be wrapped.
func (r *Runner) terminalWidth() (int, bool) {
	if r.Format != output.FormatTable {
		return 0, false
	}

	widthFunc := r.TerminalWidth
	if widthFunc == nil {
		widthFunc = stdoutTerminalWidth
	}

	width, ok := widthFunc()
	if !ok || width <= 0 {
		return 0, false
	}

	return width, true
}

// wrapText word-wraps text so that no line is wider than width terminal display cells. Existing line breaks are
// preserved, runs of whitespace between words are collapsed, and words longer than width are split.
func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}

	paragraphs := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		wrapped = append(wrapped, wrapParagraph(paragraph, width)...)
	}

	return strings.Join(wrapped, "\n")
}

func wrapParagraph(paragraph string, width int) []string {
	if displayWidth(paragraph) <= width {
		return []string{paragraph}
	}

	words := strings.Fields(paragraph)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{}
	var current strings.Builder
	currentWidth := 0
	flush := func() {
		lines = append(lines, current.String())
		current.Reset()
		currentWidth = 0
	}

	for _, word := range words {
		for _, chunk := range splitWord(word, width) {
			chunkWidth := displayWidth(chunk)
			if currentWidth > 0 && currentWidth+1+chunkWidth > width {
				flush()
			}
			if currentWidth > 0 {
				current.WriteByte(' ')
				currentWidth++
			}
			current.WriteString(chunk)
			currentWidth += chunkWidth
		}
	}
	flush()

	return lines
}

// splitWord breaks a word wider than width display cells into chunks that each fit within width.
// Chunks break only on grapheme cluster boundaries, so wide and combined characters are never split.
// A single grapheme wider than width is placed on its own line because it cannot be split further.
func splitWord(word string, width int) []string {
	if displayWidth(word) <= width {
		return []string{word}
	}

	chunks := []string{}
	for _, chunk := range strings.Split(ansi.Hardwrap(word, width, false), "\n") {
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
	}

	return chunks
}

// displayWidth returns the number of terminal cells s occupies, counting wide characters such as CJK
// as two cells and combining characters as zero.
func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

// descriptionColumnOffset returns the widest display column at which a DESCRIPTION cell can start in the
// property table, mirroring how output.TableFormatter lays out columns with text/tabwriter.
//
// text/tabwriter sizes and pads columns by rune count rather than display width. Continuation lines have
// empty leading cells, so they start at the rune-based column offset and stay aligned with each other. A
// row whose NAME or TYPE contains wide characters (for example CJK) is shifted right by the extra display
// cells; the offset returned here accounts for that shift, so no wrapped line exceeds the terminal width,
// but that row's first line is not aligned with its continuation lines. Property names and types are
// ASCII in practice, so this only affects unusual schemas.
func descriptionColumnOffset(schemaList []FieldSchema) int {
	rows := [][]string{{"NAME", "TYPE", "REQUIRED", "READ-ONLY"}}
	for _, field := range schemaList {
		rows = append(rows, []string{field.Name, field.Type, strconv.FormatBool(field.IsRequired), strconv.FormatBool(field.IsReadOnly)})
	}

	// Column widths as text/tabwriter computes them, in runes.
	columnWidths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			columnWidths[i] = max(columnWidths[i], utf8.RuneCountInString(cell))
		}
	}
	for i := range columnWidths {
		columnWidths[i] = max(columnWidths[i]+output.TablePadSize, output.TableColumnMinWidth)
	}

	// Continuation lines have empty leading cells and start at the sum of the column widths.
	offset := 0
	for _, width := range columnWidths {
		offset += width
	}

	// Each cell occupies its display width plus the padding tabwriter adds based on its rune count.
	for _, row := range rows {
		rowOffset := 0
		for i, cell := range row {
			rowOffset += displayWidth(cell) + columnWidths[i] - utf8.RuneCountInString(cell)
		}
		offset = max(offset, rowOffset)
	}

	return offset
}

// wrapDescriptions returns a copy of schemaList with each description wrapped so the table fits within
// terminalWidth. Continuation lines are rendered by the table formatter under the DESCRIPTION column.
func wrapDescriptions(schemaList []FieldSchema, terminalWidth int) []FieldSchema {
	descriptionWidth := max(terminalWidth-descriptionColumnOffset(schemaList), minDescriptionWidth)

	wrapped := make([]FieldSchema, len(schemaList))
	for i, field := range schemaList {
		wrapped[i] = field
		wrapped[i].Description = wrapText(field.Description, descriptionWidth)
	}

	return wrapped
}
