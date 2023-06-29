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
	"fmt"
	"io"
)

// Interface is a mockable interface for writing cli output
type Interface interface {
	// LogInfo logs a message that is displayed to the user by default.
	LogInfo(format string, v ...any)

	// Write
	WriteFormatted(format string, obj any, options FormatterOptions) error
}

type OutputWriter struct {
	Writer io.Writer
}

// # Function Explanation
//
// LogInfo takes in a format string and a variable number of arguments and prints the formatted string to the Writer.
func (o *OutputWriter) LogInfo(format string, v ...any) {
	fmt.Fprintf(o.Writer, format, v...)
	fmt.Fprintln(o.Writer)
}

// # Function Explanation
//
// WriteFormatted takes in a format string, an object of any type, and a FormatterOptions object, and calls
// the Write function with the given parameters, writing the output to the Writer field of the OutputWriter object.
func (o *OutputWriter) WriteFormatted(format string, obj any, options FormatterOptions) error {
	return Write(format, obj, o.Writer, options)
}

// # Function Explanation
//
// Write takes in a format string, an object, a writer and formatter options and attempts to format the object according
// to the format string and write it to the writer. If an error occurs during formatting or writing, an error is returned.
func Write(format string, obj any, writer io.Writer, options FormatterOptions) error {
	formatter, err := NewFormatter(format)
	if err != nil {
		return err
	}

	err = formatter.Format(obj, writer, options)
	if err != nil {
		return err
	}

	return nil
}
