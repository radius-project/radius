// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"fmt"
	"io"
	"strings"
)

type FormatterOptions struct {
	// Columns used for table formatting
	Columns []Column
}

type Column struct {
	Heading     string
	JSONPath    string
	Transformer func(string) string
}

type Formatter interface {
	Format(obj interface{}, writer io.Writer, options FormatterOptions) error
}

func NewFormatter(format string) (Formatter, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	switch normalized {
	case FormatJson:
		return &JSONFormatter{}, nil
	case FormatList:
		return &ListFormatter{}, nil
	case FormatTable:
		return &TableFormatter{}, nil
	default:
		return nil, fmt.Errorf("unsupported format %s", format)
	}
}
