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
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/radius-project/radius/pkg/cli/cmd/resourcetype/common"
	"github.com/radius-project/radius/pkg/cli/manifest"
	"github.com/radius-project/radius/pkg/cli/output"
	"github.com/radius-project/radius/pkg/cli/workspaces"
	"github.com/stretchr/testify/require"
)

const longSizeDescription = "The size of the PostgreSQL database, non production environments can be (S)mall or (M)edium, " +
	"production environments can be or (S)mall, (M)edium, (L)arge, or (XL)arge"

func fixedWidth(width int) TerminalWidthFunc {
	return func() (int, bool) { return width, true }
}

func noTerminal() (int, bool) { return 0, false }

func Test_wrapText(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{
			name:     "empty",
			text:     "",
			width:    10,
			expected: "",
		},
		{
			name:     "short text is unchanged",
			text:     "The name of the database",
			width:    40,
			expected: "The name of the database",
		},
		{
			name:     "text equal to width is unchanged",
			text:     "abcde fghij",
			width:    11,
			expected: "abcde fghij",
		},
		{
			name:     "wraps at word boundaries",
			text:     "The quick brown fox jumps over the lazy dog",
			width:    15,
			expected: "The quick brown\nfox jumps over\nthe lazy dog",
		},
		{
			name:     "collapses whitespace when wrapping",
			text:     "alpha   beta\tgamma  delta",
			width:    11,
			expected: "alpha beta\ngamma delta",
		},
		{
			name:     "preserves existing line breaks",
			text:     "first line\nsecond line that is long",
			width:    12,
			expected: "first line\nsecond line\nthat is long",
		},
		{
			name:     "splits long unbreakable tokens",
			text:     "see https://example.com/a/very/long/path for details",
			width:    10,
			expected: "see\nhttps://ex\nample.com/\na/very/lon\ng/path for\ndetails",
		},
		{
			name:     "splits multi-byte runes safely",
			text:     "ééééééé",
			width:    3,
			expected: "ééé\nééé\né",
		},
		{
			name:     "wraps wide characters by display width",
			text:     "数据库服务器名称数据库",
			width:    8,
			expected: "数据库服\n务器名称\n数据库",
		},
		{
			name:     "wide text that fits in runes but not in cells is wrapped",
			text:     "这是一个很长的描述",
			width:    10,
			expected: "这是一个很\n长的描述",
		},
		{
			name:     "wraps mixed ASCII and wide words",
			text:     "The 数据库 name is 服务器名称",
			width:    12,
			expected: "The 数据库\nname is\n服务器名称",
		},
		{
			name:     "never splits a wide rune across lines",
			text:     "a数据库",
			width:    4,
			expected: "a数\n据库",
		},
		{
			name:     "never splits a grapheme cluster",
			text:     "👩‍💻👩‍💻👩‍💻",
			width:    5,
			expected: "👩‍💻👩‍💻\n👩‍💻",
		},
		{
			name:     "non-positive width is unchanged",
			text:     "some text",
			width:    0,
			expected: "some text",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.expected, wrapText(tc.text, tc.width))
		})
	}
}

func Test_descriptionColumnOffset(t *testing.T) {
	t.Parallel()

	t.Run("uses column minimum widths", func(t *testing.T) {
		t.Parallel()
		// NAME=10, TYPE=10, REQUIRED=10, READ-ONLY=11
		require.Equal(t, 41, descriptionColumnOffset([]FieldSchema{{Name: "a", Type: "string"}}))
	})

	t.Run("grows with longer cells", func(t *testing.T) {
		t.Parallel()
		// NAME=13 (application+2), TYPE=10, REQUIRED=10, READ-ONLY=11
		require.Equal(t, 44, descriptionColumnOffset([]FieldSchema{{Name: "application", Type: "string"}}))
	})

	t.Run("accounts for tabwriter padding wide cells by rune count", func(t *testing.T) {
		t.Parallel()
		// Columns are 41 runes wide, but the "数" row occupies one extra display cell before DESCRIPTION.
		require.Equal(t, 42, descriptionColumnOffset([]FieldSchema{{Name: "ab", Type: "string"}, {Name: "数", Type: "string"}}))
	})
}

func Test_wrapText_WideCharacters(t *testing.T) {
	t.Parallel()

	texts := []string{
		strings.Repeat("数据库服务器", 10),
		"PostgreSQL 数据库服务器 with a very long 描述文字描述文字描述文字描述文字 and more ASCII text",
		"https://example.com/" + strings.Repeat("数据", 30),
		strings.Repeat("é", 50) + " " + strings.Repeat("👩‍💻", 20),
	}

	for i, text := range texts {
		for _, width := range []int{2, 3, 7, 20, 33} {
			t.Run(fmt.Sprintf("text %d at %d cells", i, width), func(t *testing.T) {
				t.Parallel()

				wrapped := wrapText(text, width)
				for _, line := range strings.Split(wrapped, "\n") {
					require.True(t, utf8.ValidString(line), "line contains a split rune: %q", line)
					require.LessOrEqual(t, displayWidth(line), width, "line exceeds width: %q", line)
				}
				// Wrapping only replaces spaces with line breaks, so no character is split or lost.
				require.Equal(t, strings.Join(strings.Fields(text), ""), strings.Join(strings.Fields(wrapped), ""))
			})
		}
	}
}

// renderTable renders schemaList the same way `rad resource-type show` does.
func renderTable(t *testing.T, schemaList []FieldSchema) string {
	t.Helper()

	buf := &bytes.Buffer{}
	formatter := &output.TableFormatter{}
	err := formatter.Format(schemaList, buf, common.GetResourceTypeShowSchemaTableFormat())
	require.NoError(t, err)
	return buf.String()
}

func Test_wrapDescriptions(t *testing.T) {
	t.Parallel()

	schemaList := []FieldSchema{
		{Name: "database", Type: "string", Description: "The name of the database", IsReadOnly: true},
		{Name: "size", Type: "string", Description: longSizeDescription},
		{Name: "url", Type: "string", Description: "Docs: https://example.com/" + strings.Repeat("x", 120)},
	}

	t.Run("short descriptions are not wrapped", func(t *testing.T) {
		t.Parallel()
		wrapped := wrapDescriptions(schemaList[:1], 119)
		require.Equal(t, schemaList[:1], wrapped)
	})

	t.Run("does not modify the input", func(t *testing.T) {
		t.Parallel()
		_ = wrapDescriptions(schemaList, 60)
		require.Equal(t, longSizeDescription, schemaList[1].Description)
	})

	for _, width := range []int{119, 80, 70} {
		t.Run(fmt.Sprintf("long descriptions are wrapped and aligned at width %d", width), func(t *testing.T) {
			t.Parallel()

			offset := descriptionColumnOffset(schemaList)
			rendered := renderTable(t, wrapDescriptions(schemaList, width))
			lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
			require.Greater(t, len(lines), len(schemaList)+1)

			for _, line := range lines {
				require.LessOrEqual(t, displayWidth(line), width, "line exceeds terminal width: %q", line)
				if strings.TrimSpace(line[:offset]) == "" {
					// Continuation lines are indented to the DESCRIPTION column.
					require.NotEqual(t, ' ', rune(line[offset]), "continuation line is misaligned: %q", line)
				}
			}
			require.Contains(t, rendered, "NAME      TYPE      REQUIRED  READ-ONLY  DESCRIPTION")
		})
	}

	t.Run("matches the desired output from the issue", func(t *testing.T) {
		t.Parallel()

		list := []FieldSchema{
			{Name: "application", Type: "string"},
			{Name: "size", Type: "string", Description: longSizeDescription},
		}
		expected := "" +
			"NAME         TYPE      REQUIRED  READ-ONLY  DESCRIPTION\n" +
			"application  string    false     false      \n" +
			"size         string    false     false      The size of the PostgreSQL database, non production environments can be\n" +
			"                                            (S)mall or (M)edium, production environments can be or (S)mall, (M)edium,\n" +
			"                                            (L)arge, or (XL)arge\n"
		require.Equal(t, expected, renderTable(t, wrapDescriptions(list, 119)))
	})

	t.Run("wide descriptions are wrapped by display width and aligned", func(t *testing.T) {
		t.Parallel()

		list := []FieldSchema{
			{Name: "name", Type: "string", Description: strings.Repeat("数据库服务器名称", 8)},
			{Name: "token", Type: "string", Description: "见 https://example.com/" + strings.Repeat("文档", 40)},
		}
		const width = 80
		offset := descriptionColumnOffset(list)
		rendered := renderTable(t, wrapDescriptions(list, width))
		lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
		require.Greater(t, len(lines), len(list)+1)

		for _, line := range lines {
			require.LessOrEqual(t, displayWidth(line), width, "line exceeds terminal width: %q", line)
			if strings.TrimSpace(line[:offset]) == "" {
				require.NotEqual(t, ' ', rune(line[offset]), "continuation line is misaligned: %q", line)
			}
		}
	})

	t.Run("wide property names never push lines past the terminal width", func(t *testing.T) {
		t.Parallel()

		list := []FieldSchema{
			{Name: "ab", Type: "string", Description: longSizeDescription},
			{Name: "数据库", Type: "string", Description: longSizeDescription},
		}
		const width = 80
		for _, line := range strings.Split(strings.TrimSuffix(renderTable(t, wrapDescriptions(list, width)), "\n"), "\n") {
			require.LessOrEqual(t, displayWidth(line), width, "line exceeds terminal width: %q", line)
		}
	})

	t.Run("narrow terminals wrap at a minimum width", func(t *testing.T) {
		t.Parallel()

		rendered := renderTable(t, wrapDescriptions(schemaList[1:2], 30))
		offset := descriptionColumnOffset(schemaList[1:2])
		for _, line := range strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")[1:] {
			require.LessOrEqual(t, displayWidth(line), offset+minDescriptionWidth)
		}
	})
}

func Test_display_Wrapping(t *testing.T) {
	t.Parallel()

	longResourceDescription := strings.TrimSpace(strings.Repeat("A standard PostgreSQL database server. ", 5))
	resourceType := common.ResourceType{
		Name:                      "MyCompany.Resources/testResources",
		ResourceProviderNamespace: "MyCompany.Resources",
		Description:               longResourceDescription,
		APIVersions: map[string]*common.APIVersionProperties{"2023-10-01-preview": {
			Schema: map[string]any{
				"properties": map[string]any{
					"size": map[string]any{
						"type":        "string",
						"description": longSizeDescription,
					},
				},
			},
		}},
	}

	// findWrites returns the resource type description and the property table written to output.
	findWrites := func(t *testing.T, writes []any) (string, []FieldSchema) {
		t.Helper()

		description := ""
		var schema []FieldSchema
		for i, w := range writes {
			if log, ok := w.(output.LogOutput); ok && log.Format == "\nDESCRIPTION:" {
				description = writes[i+1].(output.LogOutput).Params[0].(string)
			}
			if f, ok := w.(output.FormattedOutput); ok {
				if list, ok := f.Obj.([]FieldSchema); ok {
					schema = list
				}
			}
		}
		require.NotNil(t, schema)
		return description, schema
	}

	testcases := []struct {
		name                string
		format              string
		width               TerminalWidthFunc
		expectedDescription string
		expectedProperty    string
	}{
		{
			name:                "table output on a terminal is wrapped",
			format:              output.FormatTable,
			width:               fixedWidth(80),
			expectedDescription: wrapText(longResourceDescription, 80),
			expectedProperty:    wrapText(longSizeDescription, 80-41),
		},
		{
			name:                "narrow terminal wraps at the minimum description width",
			format:              output.FormatTable,
			width:               fixedWidth(30),
			expectedDescription: wrapText(longResourceDescription, 30),
			expectedProperty:    wrapText(longSizeDescription, minDescriptionWidth),
		},
		{
			name:                "output is unchanged when stdout is not a terminal",
			format:              output.FormatTable,
			width:               noTerminal,
			expectedDescription: longResourceDescription,
			expectedProperty:    longSizeDescription,
		},
		{
			name:                "output is unchanged for non-table formats",
			format:              output.FormatJson,
			width:               fixedWidth(80),
			expectedDescription: longResourceDescription,
			expectedProperty:    longSizeDescription,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sink := &output.MockOutput{}
			runner := &Runner{Format: tc.format, Output: sink, TerminalWidth: tc.width}
			require.NoError(t, runner.display(&resourceType))

			description, schema := findWrites(t, sink.Writes)
			require.Equal(t, tc.expectedDescription, description)
			require.Len(t, schema, 1)
			require.Equal(t, tc.expectedProperty, schema[0].Description)
		})
	}

	t.Run("source schema is not modified", func(t *testing.T) {
		t.Parallel()

		runner := &Runner{Format: output.FormatTable, Output: &output.MockOutput{}, TerminalWidth: fixedWidth(40)}
		require.NoError(t, runner.display(&resourceType))
		require.Equal(t, longResourceDescription, resourceType.Description)
	})
}

func Test_Run_JSONIsNotWrapped(t *testing.T) {
	t.Parallel()

	clientFactory, err := manifest.NewTestClientFactory(manifest.WithResourceProviderServerNoError)
	require.NoError(t, err)

	sink := &output.MockOutput{}
	runner := &Runner{
		UCPClientFactory: clientFactory,
		Workspace: &workspaces.Workspace{
			Connection: map[string]any{"kind": "kubernetes", "context": "kind-kind"},
			Name:       "kind-kind",
			Scope:      "/planes/radius/local/resourceGroups/test-group",
		},
		Format:                    output.FormatJson,
		Output:                    sink,
		TerminalWidth:             fixedWidth(10),
		ResourceTypeName:          "MyCompany.Resources/testResources",
		ResourceProviderNamespace: "MyCompany.Resources",
		ResourceTypeSuffix:        "testResources",
	}

	require.NoError(t, runner.Run(t.Context()))
	require.Len(t, sink.Writes, 1)
	formatted := sink.Writes[0].(output.FormattedOutput)
	require.Equal(t, output.FormatJson, formatted.Format)
	require.Equal(t, "Resource type description", formatted.Obj.(common.ResourceType).Description)
}
