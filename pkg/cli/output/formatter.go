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
	Heading  string
	JSONPath string
}

type Formatter interface {
	Format(obj interface{}, writer io.Writer, options FormatterOptions) error
}

func NewFormatter(format string) (Formatter, error) {
	if strings.EqualFold(format, FormatJson) {
		return &JSONFormatter{}, nil
	} else if strings.EqualFold(format, FormatTable) {
		return &TableFormatter{}, nil
	} else {
		return nil, fmt.Errorf("unsupported format %s", format)
	}
}
