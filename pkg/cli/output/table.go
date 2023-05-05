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
	"strings"
	"text/tabwriter"

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

type TableFormatter struct {
}

// # Function Explanation
// 
//	TableFormatter's Format function takes in an object, a writer and formatting options, and writes a table to the writer 
//	using the formatting options. It checks if columns were defined, converts the object to a slice, and then parses and 
//	transforms the columns before writing them to the writer. If any errors occur, it returns an error.
func (f *TableFormatter) Format(obj any, writer io.Writer, options FormatterOptions) error {
	if len(options.Columns) == 0 {
		return errors.New("no columns were defined, table format is not supported for this command")
	}

	rows, err := convertToSlice(obj)
	if err != nil {
		return err
	}

	headings := []string{}
	parsers := []*jsonpath.JSONPath{}
	transformers := []func(string) string{}
	for _, c := range options.Columns {
		headings = append(headings, c.Heading)

		p := jsonpath.New(c.Heading).AllowMissingKeys(true)
		err := p.Parse(c.JSONPath)
		if err != nil {
			return err
		}

		parsers = append(parsers, p)
		transformers = append(transformers, c.Transformer)
	}

	tabs := tabwriter.NewWriter(writer, TableColumnMinWidth, TableTabSize, TablePadSize, TablePadCharacter, TableFlags)
	_, err = tabs.Write([]byte(strings.Join(headings, "\t") + "\n"))
	if err != nil {
		return err
	}

	for _, row := range rows {

		// For each row evaluate the path and write to output using tab as separator
		for i, p := range parsers {
			transformer := transformers[i]
			if transformer != nil {
				buf := bytes.Buffer{}
				err := p.Execute(&buf, row)
				if err != nil {
					return err
				}
				_, err = tabs.Write([]byte(transformer(buf.String())))
				if err != nil {
					return err
				}
			} else {
				err := p.Execute(tabs, row)
				if err != nil {
					return err
				}
			}
			if i < len(parsers)-1 {
				_, err = tabs.Write([]byte("\t"))
				if err != nil {
					return err
				}
			}
		}

		_, err := tabs.Write([]byte("\n"))
		if err != nil {
			return err
		}
	}

	err = tabs.Flush()
	if err != nil {
		return err
	}

	return nil
}

var _ Formatter = (*TableFormatter)(nil)
