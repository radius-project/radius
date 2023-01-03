// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

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
