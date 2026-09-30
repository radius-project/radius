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

// wrapText word-wraps text so that no line is longer than width runes. Existing line breaks are
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
	if utf8.RuneCountInString(paragraph) <= width {
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
			chunkWidth := utf8.RuneCountInString(chunk)
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

// splitWord breaks a word longer than width runes into width-sized chunks.
func splitWord(word string, width int) []string {
	runes := []rune(word)
	if len(runes) <= width {
		return []string{word}
	}

	chunks := make([]string, 0, (len(runes)+width-1)/width)
	for len(runes) > width {
		chunks = append(chunks, string(runes[:width]))
		runes = runes[width:]
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}

	return chunks
}

// descriptionColumnOffset returns the column where the DESCRIPTION column starts in the property table,
// mirroring how output.TableFormatter sizes columns with text/tabwriter.
func descriptionColumnOffset(schemaList []FieldSchema) int {
	widths := []int{
		utf8.RuneCountInString("NAME"),
		utf8.RuneCountInString("TYPE"),
		utf8.RuneCountInString("REQUIRED"),
		utf8.RuneCountInString("READ-ONLY"),
	}
	for _, field := range schemaList {
		cells := []string{field.Name, field.Type, strconv.FormatBool(field.IsRequired), strconv.FormatBool(field.IsReadOnly)}
		for i, cell := range cells {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	offset := 0
	for _, width := range widths {
		offset += max(width+output.TablePadSize, output.TableColumnMinWidth)
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
