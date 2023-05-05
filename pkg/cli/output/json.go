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

// # Function Explanation
// 
//	JSONFormatter's Format function takes an object and a writer, and writes the object in JSON format to the writer, with 
//	indentation. It also handles any errors that may occur while writing to the writer.
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
