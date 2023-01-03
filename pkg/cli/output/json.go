// ------------------------------------------------------------
// Copyright (c) Microsoft Corporation.
// Licensed under the MIT License.
// ------------------------------------------------------------

package output

import (
	"encoding/json"
	"io"
)

type JSONFormatter struct {
}

func (f *JSONFormatter) Format(obj any, writer io.Writer, options FormatterOptions) error {
	b, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return err
	}

	_, err = writer.Write(b)
	if err != nil {
		return err
	}

	_, err = writer.Write([]byte("\n"))
	if err != nil {
		return err
	}

	return nil
}

var _ Formatter = (*JSONFormatter)(nil)
